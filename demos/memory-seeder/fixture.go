// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bufio"
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strconv"
	"strings"
	"time"
)

// fixtures holds generated corpora the seeder replays with --fixture. Each is
// a directory written by demos/memory-galaxy: corpus.jsonl (user,
// user_for_agent and agent tiers) and graph.json (the institutional tier).
// Embedded, so a seed needs no volume and the image is the whole input.
//
//go:embed all:fixtures
var fixtures embed.FS

// fixtureMemory is one corpus.jsonl line (demos/memory-galaxy Memory).
type fixtureMemory struct {
	ID       string `json:"id"`
	Tier     string `json:"tier"`
	Category string `json:"category"`
	Theme    string `json:"theme"`
	Subtheme string `json:"subtheme"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	Member   string `json:"member"`
	Agent    string `json:"agent"`
}

type fixtureGraph struct {
	Items []struct {
		Key      string `json:"key"`
		Kind     string `json:"kind"`
		Title    string `json:"title"`
		Content  string `json:"content"`
		Category string `json:"category"`
	} `json:"items"`
	Edges []struct {
		From     string `json:"from"`
		FromKind string `json:"from_kind"`
		To       string `json:"to"`
		ToKind   string `json:"to_kind"`
		Type     string `json:"type"`
	} `json:"edges"`
}

type fixture struct {
	name     string
	memories []fixtureMemory
	graph    fixtureGraph
}

// fixtureKind is the about kind every replayed memory carries, keyed by its
// corpus id: a second seed of the same corpus supersedes each memory in place
// instead of adding a copy.
const fixtureKind = "galaxy"

// memberPrefix makes a corpus member (m0042) into the raw user id the seeder
// pseudonymises, exactly as the facade would for that identity.
const memberPrefix = "larkmere-member-"

func loadFixture(name string) (fixture, error) {
	f := fixture{name: name}
	dir := path.Join("fixtures", name)
	corpus, err := fs.ReadFile(fixtures, path.Join(dir, "corpus.jsonl"))
	if err != nil {
		return f, fmt.Errorf("fixture %q: %w", name, err)
	}
	sc := bufio.NewScanner(bytes.NewReader(corpus))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for line := 1; sc.Scan(); line++ {
		if len(bytes.TrimSpace(sc.Bytes())) == 0 {
			continue
		}
		var m fixtureMemory
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			return f, fmt.Errorf("fixture %q corpus line %d: %w", name, line, err)
		}
		f.memories = append(f.memories, m)
	}
	if err := sc.Err(); err != nil {
		return f, err
	}
	graph, err := fs.ReadFile(fixtures, path.Join(dir, "graph.json"))
	if err != nil {
		return f, fmt.Errorf("fixture %q: %w", name, err)
	}
	if err := json.Unmarshal(graph, &f.graph); err != nil {
		return f, fmt.Errorf("fixture %q graph: %w", name, err)
	}
	return f, nil
}

// agentNames lists the agents the corpus writes for, sorted, so a missing
// --agent mapping is reported all at once.
func (f fixture) agentNames() []string {
	set := map[string]bool{}
	for _, m := range f.memories {
		if m.Agent != "" {
			set[m.Agent] = true
		}
	}
	names := make([]string, 0, len(set))
	for n := range set {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func (f fixture) byTier(tier string) []fixtureMemory {
	var out []fixtureMemory
	for _, m := range f.memories {
		if m.Tier == tier {
			out = append(out, m)
		}
	}
	return out
}

func category(c string) string { return "memory:" + c }

func about(id string) map[string]string { return map[string]string{"kind": fixtureKind, "key": id} }

// SaveFixtureUser writes a user or user_for_agent memory. agentUID is empty
// for the user tier.
func (c *Client) SaveFixtureUser(ctx context.Context, m fixtureMemory, agentUID string) (string, error) {
	scope := map[string]string{
		fieldWorkspaceID:   c.workspaceUID,
		fieldVirtualUserID: c.pseudo.ID(memberPrefix + m.Member),
	}
	if agentUID != "" {
		scope[fieldAgentID] = agentUID
	}
	body := map[string]any{
		fieldType: m.Type, fieldContent: m.Content, fieldConfidence: 0.8,
		"category": category(m.Category), "scope": scope, "about": about(m.ID),
		fieldMetadata: map[string]any{fieldProvenance: "user_requested", "theme": m.Theme, "subtheme": m.Subtheme},
	}
	return c.saveID(ctx, pathSaveMemory, body)
}

// SaveFixtureAgent writes an agent-tier memory for the agent with agentUID.
func (c *Client) SaveFixtureAgent(ctx context.Context, m fixtureMemory, agentUID string) (string, error) {
	body := map[string]any{
		fieldWorkspaceID: c.workspaceUID, fieldAgentID: agentUID,
		fieldType: m.Type, fieldContent: m.Content, fieldConfidence: 0.8,
		"category": category(m.Category),
		// The agent route reads its about key from metadata, not an about
		// field (Omnia memory-api itemFromMemory).
		fieldMetadata: map[string]any{"theme": m.Theme, "subtheme": m.Subtheme,
			"about_kind": fixtureKind, "about_key": m.ID},
		"expires_at": agentExpiry(m.ID, time.Now()).Format(time.RFC3339),
	}
	return c.saveID(ctx, pathAgentMemories, body)
}

// Agent memories expire between agentExpiryMin and agentExpiryMax after the
// seed, spread by a hash of the memory's id: the same memory gets the same
// lifetime on every seed, and the tier doesn't lapse all on one day. The
// chart's agent tier caps expiry at 730 days (galaxy.retention), above
// agentExpiryMax. The user save route takes no expires_at, so user memories
// take their category's TTL from the policy instead.
const (
	agentExpiryMin = 120 * 24 * time.Hour
	agentExpiryMax = 720 * 24 * time.Hour
)

func agentExpiry(id string, now time.Time) time.Time {
	h := fnv.New64a()
	_, _ = h.Write([]byte(id))
	span := uint64((agentExpiryMax - agentExpiryMin) / time.Hour)
	return now.Add(agentExpiryMin + time.Duration(h.Sum64()%span)*time.Hour)
}

// fixtureSource names the corpus's institutional rows.
func fixtureSource(name string) string { return "omnia-demo-" + name }

// IngestGraph sends the institutional tier: every item, then every edge, in
// one batch under the fixture's source (memory-api takes up to 5000 items).
// An edge whose ends are not held is rejected, never dropped, and fails here.
func (c *Client) IngestGraph(ctx context.Context, name string, g fixtureGraph) (int, error) {
	items := make([]map[string]any, 0, len(g.Items))
	for _, it := range g.Items {
		item := map[string]any{
			"about": map[string]string{"kind": it.Kind, "key": it.Key},
			"kind":  it.Kind, "title": it.Title, "content": it.Content,
		}
		if it.Category != "" {
			item[fieldMetadata] = map[string]any{"category": category(it.Category)}
		}
		items = append(items, item)
	}
	edges := make([]map[string]any, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, map[string]any{
			"from": map[string]string{"kind": e.FromKind, "key": e.From},
			"to":   map[string]string{"kind": e.ToKind, "key": e.To},
			"type": e.Type,
		})
	}
	body := map[string]any{fieldWorkspaceID: c.workspaceUID, "source": fixtureSource(name), "items": items, "edges": edges}
	raw, err := c.postJSON(ctx, pathIngestBatch, body, http.StatusAccepted)
	if err != nil {
		return 0, err
	}
	var r ingestBatchResponse
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &r); err != nil {
			return 0, fmt.Errorf("%s: decode response: %w", pathIngestBatch, err)
		}
	}
	if len(r.Rejected) > 0 {
		var why []string
		for _, rj := range r.Rejected {
			why = append(why, rj.Key+": "+rj.Error)
		}
		return r.Accepted + r.Skipped, fmt.Errorf("%d graph entries rejected: %s", len(r.Rejected), strings.Join(why, "; "))
	}
	return r.Accepted + r.Skipped, nil
}

// runFixture replays a fixture: the institutional graph first (the user and
// agent tiers point at nothing in it, so order is only for the log), then the
// agent, user_for_agent and user tiers, then the Galaxy build.
func runFixture(ctx context.Context, c *Client, f fixture, agents map[string]string, opt options) (summary, error) {
	r := &runner{c: c, refused: map[string]string{}}

	graph := stepResult{Name: "graph", Items: len(f.graph.Items) + len(f.graph.Edges)}
	n, err := c.IngestGraph(ctx, f.name, f.graph)
	graph.Stored = n
	if err != nil {
		if _, fatal := r.settle(&graph, tierIngest, err); fatal != nil {
			r.sum.Steps = append(r.sum.Steps, graph)
			return r.sum, fatal
		}
	}
	r.sum.Steps = append(r.sum.Steps, graph)

	if opt.privacyAPI != "" {
		grants := f.consentGrants()
		members := make([]string, 0, len(grants))
		for m := range grants {
			members = append(members, m)
		}
		sort.Strings(members)
		consent := stepResult{Name: "consent grants", Items: len(members)}
		for _, m := range members {
			if err := c.GrantConsent(ctx, opt.privacyAPI, m, grants[m]); err != nil {
				consent.Failed++
				log.Printf("consent grant for %s: %v", m, err)
				continue
			}
			consent.Stored++
		}
		r.sum.Steps = append(r.sum.Steps, consent)
	}

	agentMems, forAgent, user := f.byTier("agent"), f.byTier("user_for_agent"), f.byTier("user")
	steps := []struct {
		name, tier string
		n          int
		write      func(i int) (string, error)
	}{
		{stepAgent, tierAgent, len(agentMems), func(i int) (string, error) {
			return c.SaveFixtureAgent(ctx, agentMems[i], agents[agentMems[i].Agent])
		}},
		{"user_for_agent", tierUser, len(forAgent), func(i int) (string, error) {
			return c.SaveFixtureUser(ctx, forAgent[i], agents[forAgent[i].Agent])
		}},
		{stepUser, tierUser, len(user), func(i int) (string, error) {
			before := c.Suppressed
			id, err := c.SaveFixtureUser(ctx, user[i], "")
			if c.Suppressed > before {
				log.Printf("consent refused %s (%s, member %s)", user[i].ID, user[i].Category, user[i].Member)
			}
			return id, err
		}},
	}
	for _, st := range steps {
		if err := r.writes(st.name, st.tier, st.n, st.write); err != nil {
			return r.sum, err
		}
	}

	// Forget the agent memories an earlier corpus wrote and this one no
	// longer has, so a regenerated corpus replaces the last one instead of
	// accumulating beside it.
	keep, corpusText := map[string]bool{}, map[string]bool{}
	for _, m := range agentMems {
		keep[m.ID] = true
		corpusText[m.Content] = true
	}
	prune := stepResult{Name: "pruned stale agent memories"}
	for _, name := range f.agentNames() {
		n, err := c.PruneAgent(ctx, agents[name], keep, corpusText)
		if err != nil {
			log.Printf("prune %s: %v", name, err)
			prune.Failed++
			continue
		}
		prune.Items += n
		prune.Stored += n
	}
	r.sum.Steps = append(r.sum.Steps, prune)

	if opt.buildGalaxy {
		gal := stepResult{Name: stepGalaxy, Items: 1}
		if err := buildGalaxy(ctx, c, opt); err != nil {
			if _, fatal := r.settle(&gal, tierAdmin, err); fatal != nil {
				r.sum.Steps = append(r.sum.Steps, gal)
				return r.sum, fatal
			}
		} else {
			gal.Stored = 1
		}
		r.sum.Steps = append(r.sum.Steps, gal)
	}
	return r.sum, r.sum.Err()
}

// grantCategories are the consent categories memory-api refuses to store until
// the member grants them (Omnia privacy consent.go RequiresExplicitGrant).
var grantCategories = map[string]bool{"identity": true, "location": true, "health": true}

// consentGrants lists, per member, the categories that need a grant and that
// the member's memories use: what each member would have agreed to when they
// shared it.
func (f fixture) consentGrants() map[string][]string {
	sets := map[string]map[string]bool{}
	for _, m := range f.memories {
		if m.Member == "" || !grantCategories[m.Category] {
			continue
		}
		if sets[m.Member] == nil {
			sets[m.Member] = map[string]bool{}
		}
		sets[m.Member][category(m.Category)] = true
	}
	out := make(map[string][]string, len(sets))
	for member, set := range sets {
		for c := range set {
			out[member] = append(out[member], c)
		}
		sort.Strings(out[member])
	}
	return out
}

// GrantConsent records a member's consent grants with privacy-api, keyed on
// the member's pseudonym like every privacy route.
func (c *Client) GrantConsent(ctx context.Context, privacyBase, member string, grants []string) error {
	body, _ := json.Marshal(map[string]any{"grants": grants})
	url := strings.TrimRight(privacyBase, "/") + "/api/v1/privacy/preferences/" + c.pseudo.ID(memberPrefix+member) + "/consent"
	for attempt := 1; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if tok := c.token(); tok != "" {
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := c.http.Do(req)
		if err == nil {
			msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
			_ = resp.Body.Close()
			if resp.StatusCode/100 == 2 {
				return nil
			}
			err = fmt.Errorf("privacy-api %s: %s", resp.Status, strings.TrimSpace(string(msg)))
			if resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
				return err
			}
		}
		if attempt == maxAttempts {
			return err
		}
		if serr := c.sleep(ctx, time.Duration(attempt)*c.backoff); serr != nil {
			return serr
		}
	}
}

// PruneAgent forgets the agent's replayed memories (about kind "galaxy") whose
// corpus id is no longer in keep: what a regenerated corpus dropped or renamed.
// It also forgets key-less memories whose text is exactly a corpus memory's:
// copies replayed before the seeder put the key where the agent route reads
// it (memory-api keeps no other metadata to tell them by). Memories the agent
// learned itself match neither and are left.
func (c *Client) PruneAgent(ctx context.Context, agentUID string, keep, corpusText map[string]bool) (int, error) {
	var stale []string
	for offset := 0; ; offset += 100 {
		q := url.Values{"workspace": {c.workspaceUID}, "agent": {agentUID}, "limit": {"100"}, "offset": {strconv.Itoa(offset)}}
		raw, err := c.getJSON(ctx, pathAgentMemories+"?"+q.Encode())
		if err != nil {
			return 0, err
		}
		var page struct {
			Memories []struct {
				ID       string         `json:"id"`
				Content  string         `json:"content"`
				Metadata map[string]any `json:"metadata"`
			} `json:"memories"`
		}
		if err := json.Unmarshal(raw, &page); err != nil {
			return 0, fmt.Errorf("%s: decode: %w", pathAgentMemories, err)
		}
		for _, m := range page.Memories {
			kind, _ := m.Metadata["about_kind"].(string)
			key, _ := m.Metadata["about_key"].(string)
			switch {
			case kind == fixtureKind && !keep[key]:
				stale = append(stale, m.ID) // the corpus no longer has it
			case kind == "" && corpusText[m.Content]:
				stale = append(stale, m.ID) // replayed before agent memories carried a key
			}
		}
		if len(page.Memories) < 100 {
			break
		}
	}
	for _, id := range stale {
		q := url.Values{"workspace": {c.workspaceUID}, "agent": {agentUID}}
		if err := c.deleteReq(ctx, pathAgentMemories+"/"+url.PathEscape(id)+"?"+q.Encode()); err != nil {
			return 0, err
		}
	}
	return len(stale), nil
}

func (c *Client) getJSON(ctx context.Context, pathAndQuery string) ([]byte, error) {
	return c.simple(ctx, http.MethodGet, pathAndQuery)
}

func (c *Client) deleteReq(ctx context.Context, pathAndQuery string) error {
	_, err := c.simple(ctx, http.MethodDelete, pathAndQuery)
	return err
}

// simple sends a bodyless request to memory-api and returns a 2xx body.
func (c *Client) simple(ctx context.Context, method, pathAndQuery string) ([]byte, error) {
	if err := c.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+pathAndQuery, nil)
	if err != nil {
		return nil, err
	}
	if tok := c.token(); tok != "" {
		req.Header.Set("Authorization", "Bearer "+tok)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("%s %s: %s: %s", method, pathAndQuery, resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}
