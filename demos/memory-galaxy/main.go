// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

// Command memory-galaxy generates the Larkmere Mutual memory Galaxy corpus
// from taxonomy.yaml, once, so the demo never pays for it again.
//
// Every model call goes through cache/: a response is stored under the SHA-256
// of its request (model, system prompt, user prompt) and committed. A rerun
// reads the cache and calls the model only for prompts it has never seen, so
// rebuilding the corpus is free, and raising --scale pays only for the new
// batches (each batch is its own prompt). --offline refuses to call the model
// at all and fails on a cache miss: that is how CI and a fresh checkout
// rebuild the fixture.
//
// Output, for the memory-seeder to embed and replay:
//
//	corpus.jsonl  one memory per line: user, user_for_agent and agent tiers
//	graph.json    the institutional tier: graph nodes, guidance items, edges
//
//	go run ./demos/memory-galaxy --endpoint https://<account>.cognitiveservices.azure.com/
//	go run ./demos/memory-galaxy --offline
package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"gopkg.in/yaml.v3"
)

// Taxonomy is taxonomy.yaml.
type Taxonomy struct {
	Company    string             `yaml:"company"`
	Context    string             `yaml:"context"`
	Model      string             `yaml:"model"`
	Batch      int                `yaml:"batch"`
	Tiers      map[string]float64 `yaml:"tiers"`
	Members    int                `yaml:"members"`
	Agents     map[string]string  `yaml:"agents"`
	Categories []Category         `yaml:"categories"`
	Graph      Graph              `yaml:"graph"`
}

type Category struct {
	Name   string  `yaml:"name"`
	Themes []Theme `yaml:"themes"`
}

type Theme struct {
	Name      string     `yaml:"name"`
	Node      string     `yaml:"node"`
	Subthemes []Subtheme `yaml:"subthemes"`
}

type Subtheme struct {
	Name          string   `yaml:"name"`
	Weight        float64  `yaml:"weight"`
	Agents        []string `yaml:"agents"`
	Institutional *int     `yaml:"institutional"`
	AgentExtra    int      `yaml:"agent_extra"`
	AgentHint     string   `yaml:"agent_hint"`
}

type Graph struct {
	Nodes []Node `yaml:"nodes"`
	Edges []Edge `yaml:"edges"`
}

type Node struct {
	Key    string `yaml:"key" json:"key"`
	Kind   string `yaml:"kind" json:"kind"`
	Title  string `yaml:"title" json:"title"`
	Prompt string `yaml:"prompt" json:"-"`
}

type Edge struct {
	From string `yaml:"from"`
	To   string `yaml:"to"`
	Type string `yaml:"type"`
}

// Memory is one corpus.jsonl line.
type Memory struct {
	ID       string `json:"id"`
	Tier     string `json:"tier"`
	Category string `json:"category"`
	Theme    string `json:"theme"`
	Subtheme string `json:"subtheme"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Member   string `json:"member,omitempty"`
	Agent    string `json:"agent,omitempty"`
}

// GraphFile is graph.json: what the seeder sends to batch ingest.
type GraphFile struct {
	Items []GraphItem `json:"items"`
	Edges []GraphEdge `json:"edges"`
}

type GraphItem struct {
	Key      string `json:"key"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Content  string `json:"content"`
	Category string `json:"category,omitempty"`
}

type GraphEdge struct {
	From     string `json:"from"`
	FromKind string `json:"from_kind"`
	To       string `json:"to"`
	ToKind   string `json:"to_kind"`
	Type     string `json:"type"`
}

// job is one model call: a prompt and how its answer is used.
type job struct {
	system, user string
	key          string // cache key
	tier         string
	n            int // items to keep from the answer
	sub          *subRef
	agent        string
	batch        int
	node         *Node
}

type subRef struct {
	category, theme, node string
	s                     Subtheme
}

func main() {
	taxPath := flag.String("taxonomy", "demos/memory-galaxy/taxonomy.yaml", "taxonomy file")
	cacheDir := flag.String("cache", "demos/memory-galaxy/cache", "response cache (committed)")
	outDir := flag.String("out", "demos/memory-seeder/fixtures/larkmere-galaxy", "where corpus.jsonl and graph.json go")
	scale := flag.Float64("scale", 7, "multiplier on every count (7, the default, is the ~11k committed corpus; 1 is the ~2k review sample)")
	endpoint := flag.String("endpoint", os.Getenv("FOUNDRY_ENDPOINT"), "Foundry (Azure OpenAI) endpoint")
	offline := flag.Bool("offline", false, "never call the model; fail on a cache miss")
	workers := flag.Int("workers", 8, "concurrent model calls")
	dryRun := flag.Bool("dry-run", false, "print the counts and cache misses, call nothing, write nothing")
	flag.Parse()

	raw, err := os.ReadFile(*taxPath)
	if err != nil {
		log.Fatal(err)
	}
	var tax Taxonomy
	if err := yaml.Unmarshal(raw, &tax); err != nil {
		log.Fatalf("taxonomy: %v", err)
	}
	jobs := plan(tax, *scale)

	misses := 0
	for _, j := range jobs {
		if _, err := os.Stat(cachePath(*cacheDir, j.key)); err != nil {
			misses++
		}
	}
	counts := map[string]int{}
	for _, j := range jobs {
		counts[j.tier] += j.n
	}
	log.Printf("%d prompts (%d not cached); memories by tier: %v", len(jobs), misses, counts)
	if *dryRun {
		return
	}
	if misses > 0 && *offline {
		log.Fatalf("--offline and %d prompts are not cached", misses)
	}

	var llm *client
	if misses > 0 {
		if *endpoint == "" {
			log.Fatal("--endpoint (or FOUNDRY_ENDPOINT) is required for the uncached prompts")
		}
		llm, err = newClient(*endpoint, tax.Model)
		if err != nil {
			log.Fatal(err)
		}
	}
	answers, err := answerAll(context.Background(), jobs, *cacheDir, llm, *workers)
	if err != nil {
		log.Fatal(err)
	}
	corpus, graph, dropped := assemble(tax, jobs, answers)
	if err := write(*outDir, corpus, graph); err != nil {
		log.Fatal(err)
	}
	log.Printf("wrote %d memories and %d graph items, %d edges (%d duplicates dropped) to %s",
		len(corpus), len(graph.Items), len(graph.Edges), dropped, *outDir)
}

// plan lists every model call, in a stable order.
func plan(t Taxonomy, scale float64) []job {
	system := fmt.Sprintf("You write realistic synthetic data for a demo of an AI agent platform. "+
		"The company is fictional. %s Write in British English. Never use real people's names, real "+
		"companies other than public bodies (the Met Office, the Financial Ombudsman Service), or real "+
		"addresses. Answer with JSON only.", strings.TrimSpace(t.Context))
	batch := t.Batch
	if batch <= 0 {
		batch = 10
	}
	var jobs []job
	for _, c := range t.Categories {
		for _, th := range c.Themes {
			for _, s := range th.Subthemes {
				ref := &subRef{category: c.Name, theme: th.Name, node: th.Node, s: s}
				for _, tier := range []string{"user", "user_for_agent", "agent", "institutional"} {
					n := count(t, s, tier, scale)
					for b := 0; b*batch < n; b++ {
						agent := ""
						if (tier == "user_for_agent" || tier == "agent") && len(s.Agents) > 0 {
							agent = s.Agents[b%len(s.Agents)]
						}
						keep := min(batch, n-b*batch)
						user := prompt(t, ref, tier, agent, b, batch)
						jobs = append(jobs, job{system: system, user: user, key: hash(t.Model, system, user),
							tier: tier, n: keep, sub: ref, agent: agent, batch: b})
					}
				}
			}
		}
	}
	for i := range t.Graph.Nodes {
		nd := &t.Graph.Nodes[i]
		user := fmt.Sprintf("Write %s's document %q: %s. 120 to 220 words, in the society's own voice, as "+
			"plain text with no headings or markdown. Return {\"text\": \"...\"}.", t.Company, nd.Title, nd.Prompt)
		jobs = append(jobs, job{system: system, user: user, key: hash(t.Model, system, user), tier: "node", n: 1, node: nd})
	}
	return jobs
}

func count(t Taxonomy, s Subtheme, tier string, scale float64) int {
	if tier == "institutional" && s.Institutional != nil {
		return int(math.Round(float64(*s.Institutional) * scale))
	}
	if (tier == "agent" || tier == "user_for_agent") && len(s.Agents) == 0 {
		return 0
	}
	n := int(math.Round(s.Weight * t.Tiers[tier] * scale))
	if tier == "agent" {
		n += int(math.Round(float64(s.AgentExtra) * scale))
	}
	return n
}

// prompt is one batch's request. The batch number and its persona hint keep
// batches from repeating each other; both are part of the cache key, so a
// batch's answer never changes once it is cached.
func prompt(t Taxonomy, r *subRef, tier, agent string, b, n int) string {
	vary := personas[b%len(personas)]
	format := `Return {"items": [{"type": "fact" | "preference" | "observation", "content": "..."}]} with exactly %d items.`
	var task string
	switch tier {
	case "user":
		task = fmt.Sprintf("Write %d distinct memories an assistant at %s keeps about individual members, "+
			"from what each member said about: %s (theme: %s). One or two sentences each, in the third person "+
			"(\"The member…\", \"They…\"), each about a different member. No names. Be concrete and varied: "+
			"family, work, ages, towns in England and Wales, amounts in pounds, months. This batch leans towards %s.",
			n, t.Company, r.s.Name, r.theme, vary)
	case "user_for_agent":
		task = fmt.Sprintf("Write %d distinct notes the %s agent (%s) at %s made about one member each while "+
			"helping them with: %s. Each says what will help the next time this agent serves this member: how "+
			"they like to be dealt with, what is still open, what to check. One or two sentences, third person, "+
			"no names. This batch leans towards %s.", n, agent, t.Agents[agent], t.Company, r.s.Name, vary)
	case "agent":
		task = fmt.Sprintf("Write %d distinct lessons the %s agent (%s) at %s learned across many cases about: "+
			"%s. Each is a working pattern or tip for the next case, not about any one member, one or two "+
			"sentences, in the agent's voice (\"When…, …\").", n, agent, t.Agents[agent], t.Company, r.s.Name)
		if r.s.AgentHint != "" {
			task += " What experienced assessors have found: " + strings.TrimSpace(r.s.AgentHint)
		}
	case "institutional":
		task = fmt.Sprintf("Write %d distinct short extracts of %s's official guidance, procedures or policy "+
			"wording about: %s. Two to four sentences each, in the society's own formal voice. Use type \"fact\".",
			n, t.Company, r.s.Name)
	}
	return task + " " + fmt.Sprintf(format, n) + fmt.Sprintf(" (batch %d)", b)
}

var personas = []string{
	"younger members renting or buying their first home",
	"retired members and older homeowners",
	"families with young children",
	"members in rural Wales and the Welsh borders",
	"members in northern towns and cities",
	"self-employed members and small business owners",
	"members on tight budgets",
	"members who have been with the society for decades",
	"members in the south-west and coastal towns",
	"members caring for a relative",
}

func hash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = io.WriteString(h, p)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

func cachePath(dir, key string) string { return filepath.Join(dir, key[:2], key+".json") }

type cached struct {
	Model    string `json:"model"`
	Prompt   string `json:"prompt"`
	Response string `json:"response"`
}

// answerAll returns each job's raw answer, from the cache or the model.
func answerAll(ctx context.Context, jobs []job, dir string, llm *client, workers int) ([]string, error) {
	out := make([]string, len(jobs))
	var (
		mu    sync.Mutex
		first error
		wg    sync.WaitGroup
		done  int
	)
	todo := make(chan int)
	for w := 0; w < max(1, workers); w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range todo {
				a, err := answer(ctx, jobs[i], dir, llm)
				mu.Lock()
				out[i] = a
				done++
				if err != nil && first == nil {
					first = fmt.Errorf("prompt %d (%s, %s): %w", i, jobs[i].tier, jobs[i].key[:12], err)
				}
				if done%50 == 0 {
					log.Printf("%d/%d prompts answered", done, len(jobs))
				}
				mu.Unlock()
			}
		}()
	}
	for i := range jobs {
		todo <- i
	}
	close(todo)
	wg.Wait()
	return out, first
}

func answer(ctx context.Context, j job, dir string, llm *client) (string, error) {
	p := cachePath(dir, j.key)
	if b, err := os.ReadFile(p); err == nil {
		var c cached
		if err := json.Unmarshal(b, &c); err != nil {
			return "", fmt.Errorf("cache %s: %w", p, err)
		}
		return c.Response, nil
	}
	if llm == nil {
		return "", errors.New("not cached")
	}
	resp, err := llm.complete(ctx, j.system, j.user)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return "", err
	}
	b, _ := json.MarshalIndent(cached{Model: llm.model, Prompt: j.user, Response: resp}, "", " ")
	return resp, os.WriteFile(p, append(b, '\n'), 0o644)
}

// assemble turns the answers into the corpus and the graph, dropping
// duplicates, and assigns user-tier memories to members.
func assemble(t Taxonomy, jobs []job, answers []string) ([]Memory, GraphFile, int) {
	var corpus []Memory
	var g GraphFile
	seen := map[string]bool{}
	dropped := 0
	nodeKind := map[string]string{}
	for _, nd := range t.Graph.Nodes {
		nodeKind[nd.Key] = nd.Kind
	}
	for i, j := range jobs {
		if j.tier == "node" {
			var a struct {
				Text string `json:"text"`
			}
			if err := json.Unmarshal([]byte(answers[i]), &a); err != nil || strings.TrimSpace(a.Text) == "" {
				log.Printf("node %s: unusable answer, skipped", j.node.Key)
				continue
			}
			g.Items = append(g.Items, GraphItem{Key: j.node.Key, Kind: j.node.Kind, Title: j.node.Title, Content: strings.TrimSpace(a.Text)})
			continue
		}
		var a struct {
			Items []struct{ Type, Content string } `json:"items"`
		}
		if err := json.Unmarshal([]byte(answers[i]), &a); err != nil {
			log.Printf("prompt %s: unparsable answer, skipped", j.key[:12])
			continue
		}
		kept := 0
		for _, it := range a.Items {
			if kept == j.n {
				break
			}
			content := strings.TrimSpace(it.Content)
			norm := normalise(content)
			if content == "" || seen[norm] {
				dropped++
				continue
			}
			seen[norm] = true
			id := fmt.Sprintf("%s/%s/%s/%d-%d", j.tier, j.sub.category, slug(j.sub.s.Name), j.batch, kept)
			kept++
			if j.tier == "institutional" {
				key := "guidance-" + slug(j.sub.s.Name) + fmt.Sprintf("-%d-%d", j.batch, kept)
				g.Items = append(g.Items, GraphItem{Key: key, Kind: "guidance", Title: j.sub.s.Name, Content: content,
					Category: j.sub.category})
				if j.sub.node != "" {
					g.Edges = append(g.Edges, GraphEdge{From: key, FromKind: "guidance", To: j.sub.node, ToKind: nodeKind[j.sub.node], Type: "about"})
				}
				continue
			}
			m := Memory{ID: id, Tier: j.tier, Category: j.sub.category, Theme: j.sub.theme, Subtheme: j.sub.s.Name,
				Type: memType(it.Type), Content: content, Agent: j.agent}
			if j.tier != "agent" {
				m.Member = member(id, t.Members)
			}
			corpus = append(corpus, m)
		}
	}
	for _, e := range t.Graph.Edges {
		g.Edges = append(g.Edges, GraphEdge{From: e.From, FromKind: nodeKind[e.From], To: e.To, ToKind: nodeKind[e.To], Type: e.Type})
	}
	sort.SliceStable(g.Edges, func(a, b int) bool { return g.Edges[a].From+g.Edges[a].To < g.Edges[b].From+g.Edges[b].To })
	return corpus, g, dropped
}

func memType(t string) string {
	switch t {
	case "fact", "preference", "observation":
		return t
	}
	return "fact"
}

// member assigns a memory to one of n synthetic members, by its id, so the
// assignment survives regeneration.
func member(id string, n int) string {
	if n <= 0 {
		n = 1
	}
	h := sha256.Sum256([]byte(id))
	v := int(h[0])<<16 | int(h[1])<<8 | int(h[2])
	return fmt.Sprintf("m%04d", v%n)
}

var nonWord = regexp.MustCompile(`[^a-z0-9]+`)

func slug(s string) string {
	return strings.Trim(nonWord.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func normalise(s string) string {
	return strings.Join(strings.Fields(nonWord.ReplaceAllString(strings.ToLower(s), " ")), " ")
}

func write(dir string, corpus []Memory, g GraphFile) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, m := range corpus {
		if err := enc.Encode(m); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "corpus.jsonl"), buf.Bytes(), 0o644); err != nil {
		return err
	}
	gb, err := json.MarshalIndent(g, "", " ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "graph.json"), append(gb, '\n'), 0o644)
}

// client calls the Foundry chat completions API as the signed-in az CLI user.
type client struct {
	endpoint, model string
	cred            *azidentity.AzureCLICredential
	http            *http.Client
}

func newClient(endpoint, model string) (*client, error) {
	cred, err := azidentity.NewAzureCLICredential(nil)
	if err != nil {
		return nil, err
	}
	return &client{endpoint: strings.TrimRight(endpoint, "/"), model: model, cred: cred,
		http: &http.Client{Timeout: 3 * time.Minute}}, nil
}

func (c *client) complete(ctx context.Context, system, user string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user},
		},
		"response_format":       map[string]string{"type": "json_object"},
		"max_completion_tokens": 4000,
	})
	url := fmt.Sprintf("%s/openai/deployments/%s/chat/completions?api-version=2025-01-01-preview", c.endpoint, c.model)
	for attempt := 1; ; attempt++ {
		tok, err := c.cred.GetToken(ctx, policy.TokenRequestOptions{Scopes: []string{"https://cognitiveservices.azure.com/.default"}})
		if err != nil {
			return "", err
		}
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+tok.Token)
		resp, err := c.http.Do(req)
		if err != nil {
			if attempt < 4 {
				time.Sleep(time.Duration(attempt) * 3 * time.Second)
				continue
			}
			return "", err
		}
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			if attempt < 6 {
				time.Sleep(time.Duration(attempt) * 5 * time.Second)
				continue
			}
		}
		if resp.StatusCode != http.StatusOK {
			return "", fmt.Errorf("foundry %s: %s", resp.Status, strings.TrimSpace(string(b)))
		}
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(b, &out); err != nil || len(out.Choices) == 0 {
			return "", fmt.Errorf("foundry: unexpected body: %.200s", b)
		}
		return out.Choices[0].Message.Content, nil
	}
}
