package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strconv"
	"strings"

	"github.com/AltairaLabs/omnia-demo/demos/memory-seeder/identity"
)

// observationOwnerUser is the synthetic operations user the observations are
// attributed to. POST /api/v1/memories writes the user tier only, so every
// write there names a user; this one stands for the support team.
const observationOwnerUser = "hawkridge-ops"

// defaultLeaves is the leaf count fitted for the Galaxy, memory-api's own
// default for /admin/brain/build-leaves.
const defaultLeaves = 64

// options are the run's switches beyond the data itself.
type options struct {
	buildGalaxy bool
	leaves      int
	seed        int64
}

// Step names, as logged and reported.
const (
	stepDocuments     = "documents"
	stepInstitutional = "institutional facts"
	stepAgent         = "agent memories"
	stepUser          = "user memories"
	stepObservations  = "observations"
	stepGalaxy        = "galaxy"
)

// Tiers, which a TierRefusal skips as a whole: observations are user-tier
// writes, so a refused user tier skips them too.
const (
	tierIngest        = "institutional ingest"
	tierInstitutional = "institutional"
	tierAgent         = "agent"
	tierUser          = "user"
	tierAdmin         = "galaxy admin"
)

// institutionalFacts is how many curated institutional facts are written.
const institutionalFacts = 20

// maxLoggedFailures caps the per-write failure lines on a bad run.
const maxLoggedFailures = 8

// stepResult is what one step of the seed did.
type stepResult struct {
	Name    string
	Items   int    // writes the step had to make
	Stored  int    // writes memory-api stored (a consent-refused write is not)
	Failed  int    // writes that failed and were tolerated
	Skipped string // why memory-api refused the step's tier; "" when it ran
}

// summary is what a run did, step by step.
type summary struct {
	Steps []stepResult
}

func (s summary) failures() int {
	n := 0
	for _, st := range s.Steps {
		n += st.Failed
	}
	return n
}

// Err is the run's verdict, the rule the exit status follows. Single failed
// writes are tolerated: a demo seed should land as much as it can. But the
// run fails when
//
//   - a step that ran had something to write and stored none of it (every
//     user memory failed, say, or the Galaxy's drain or leaf fit failed); a
//     step memory-api refused as a tier (TierRefusal) is skipped, not empty;
//   - or every step was skipped, so nothing was seeded at all.
//
// A fatal refusal (FatalError) fails the run before it gets here.
func (s summary) Err() error {
	var empty []string
	ran := 0
	for _, st := range s.Steps {
		if st.Skipped != "" {
			continue
		}
		ran++
		if st.Items > 0 && st.Stored == 0 {
			empty = append(empty, st.Name)
		}
	}
	switch {
	case ran == 0:
		return errors.New("memory-api refused every step: nothing was seeded")
	case len(empty) > 0:
		return fmt.Errorf("no writes stored for: %s", strings.Join(empty, ", "))
	}
	return nil
}

// runner carries the state one seed run shares across its steps.
type runner struct {
	c       *Client
	sum     summary
	refused map[string]string // tier -> why memory-api refused it
	logged  int
}

// settle files err for one write of st. It returns a FatalError to stop the
// run, and reports whether the step must stop because memory-api refused its
// tier, which then skips every later step of that tier with one log line.
func (r *runner) settle(st *stepResult, tier string, err error) (stop bool, fatal error) {
	var fe *FatalError
	var tr *TierRefusal
	switch {
	case errors.As(err, &fe):
		return true, err
	case errors.As(err, &tr):
		r.refuse(tier, tr.Body)
		st.Skipped = r.refused[tier]
		log.Printf("skipping %s: memory-api refuses the %s tier to this caller (%s)", st.Name, tier, st.Skipped)
		return true, nil
	}
	st.Failed++
	r.logged++
	if r.logged <= maxLoggedFailures {
		log.Printf("seed write failed (%s), tolerating: %v", st.Name, err)
	}
	return false, nil
}

// refuse records why tier was refused, from the 403's body.
func (r *runner) refuse(tier, body string) {
	var e struct {
		Error string `json:"error"`
	}
	_ = json.Unmarshal([]byte(body), &e)
	why := e.Error
	switch why {
	case errEnterpriseRequired:
		why = "Omnia Enterprise is required"
	case msgSharedForbidden, msgSubjectUnverified:
		why = why + "; with internalServiceAuth on, only memory-api's exact-listed services may write it"
	}
	r.refused[tier] = why
}

// writes runs n per-item writes as one step. write reports whether
// memory-api stored the item.
func (r *runner) writes(name, tier string, n int, write func(i int) (string, error)) error {
	st := stepResult{Name: name, Items: n}
	defer func() { r.sum.Steps = append(r.sum.Steps, st) }()
	if why, ok := r.refused[tier]; ok {
		st.Skipped = why
		log.Printf("skipping %s: memory-api refused the %s tier earlier (%s)", name, tier, why)
		return nil
	}
	for i := range n {
		id, err := write(i)
		if err == nil {
			if id != "" { // "" is a consent-suppressed write
				st.Stored++
			}
			continue
		}
		if stop, fatal := r.settle(&st, tier, err); stop {
			return fatal
		}
	}
	return nil
}

// run executes the seed in dependency order: documents first, because the
// ingest routes are the ones a workspace ServiceAccount may call with
// internalServiceAuth on, then the curated, agent and user tiers, which need
// an exact-listed caller (or auth off) and are skipped when refused, then the
// Galaxy build. A refusal every later write would also get (authentication,
// a foreign workspace, the pseudonym key) stops it at once.
func run(ctx context.Context, c *Client, g Generated, opt options) (summary, error) {
	r := &runner{c: c, refused: map[string]string{}}

	docs := stepResult{Name: stepDocuments, Items: len(g.Docs)}
	n, err := c.IngestDocs(ctx, g.Docs)
	docs.Stored = n
	if err != nil {
		if _, fatal := r.settle(&docs, tierIngest, err); fatal != nil {
			r.sum.Steps = append(r.sum.Steps, docs)
			return r.sum, fatal
		}
	}
	r.sum.Steps = append(r.sum.Steps, docs)

	steps := []struct {
		name, tier string
		n          int
		write      func(i int) (string, error)
	}{
		{stepInstitutional, tierInstitutional, institutionalFacts, func(i int) (string, error) {
			return c.SaveInstitutional(ctx, typePolicy, fmt.Sprintf("Hawkridge policy fact %d", i), 0.9)
		}},
		{stepAgent, tierAgent, len(g.AgentMemories), func(i int) (string, error) {
			return c.SaveAgentMemory(ctx, g.AgentMemories[i])
		}},
		{stepUser, tierUser, len(g.UserMemories), func(i int) (string, error) {
			return c.SaveUserMemory(ctx, g.UserMemories[i])
		}},
		{stepObservations, tierUser, len(g.Observations), func(i int) (string, error) {
			return c.SaveObservation(ctx, g.Observations[i])
		}},
	}
	for _, st := range steps {
		if err := r.writes(st.name, st.tier, st.n, st.write); err != nil {
			return r.sum, err
		}
	}

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
	if f := r.sum.failures(); f > 0 {
		log.Printf("seed finished with %d tolerated write failures", f)
	}
	return r.sum, r.sum.Err()
}

// buildGalaxy embeds whatever the seed left without a vector and fits the leaf
// clusters, so the dashboard's Memory Galaxy shows the cluster tree rather
// than its flat fallback. The cluster worker builds the tree from the leaves.
func buildGalaxy(ctx context.Context, c *Client, opt options) error {
	embedded, err := c.DrainEmbeddings(ctx)
	if err != nil {
		return err
	}
	leaves, err := c.BuildLeaves(ctx, opt.leaves, opt.seed)
	if err != nil {
		return err
	}
	log.Printf("galaxy: embedded %d pending items, fitted %d leaves", embedded, leaves)
	return nil
}

// loadPseudonymizer reads the workspace pseudonym key the way Omnia's own
// workspace pods do. It never logs the key; only its fingerprint.
func loadPseudonymizer() (identity.Pseudonymizer, error) {
	key := os.Getenv(identity.KeyEnv)
	if v := os.Getenv(identity.KeyRequiredEnv); v != "" {
		required, err := strconv.ParseBool(v)
		if err != nil {
			return identity.Pseudonymizer{}, fmt.Errorf("%s=%q is not a boolean", identity.KeyRequiredEnv, v)
		}
		if required && key == "" {
			return identity.Pseudonymizer{}, fmt.Errorf("%s is required (%s=true) and is empty",
				identity.KeyEnv, identity.KeyRequiredEnv)
		}
	}
	return identity.New(key), nil
}

func main() {
	base := flag.String("memory-api", "http://localhost:8080", "memory-api base URL")
	wsUID := flag.String("workspace-uid", "", "workspace metadata.uid (required)")
	agentUID := flag.String("agent-uid", "", "AgentRuntime metadata.uid to scope agent-tier memories to (required)")
	seed := flag.Int64("seed", 1, "rng seed for deterministic output")
	tokenPath := flag.String("token-path", os.Getenv(TokenPathEnv),
		"projected ServiceAccount token file sent as a bearer token (default $"+TokenPathEnv+")")
	buildGalaxyFlag := flag.Bool("build-galaxy", true,
		"after seeding, drain pending embeddings and fit the Galaxy's leaf clusters")
	leaves := flag.Int("galaxy-leaves", defaultLeaves, "leaf clusters to fit when --build-galaxy is set")
	flag.Parse()
	if *wsUID == "" {
		log.Fatal("--workspace-uid is required " +
			"(retrieve the workspace metadata.uid from your cluster)")
	}
	if *agentUID == "" {
		log.Fatal("--agent-uid is required " +
			"(retrieve the AgentRuntime metadata.uid from your cluster)")
	}
	if *leaves < 1 || *leaves > maxLeaves {
		log.Fatalf("--galaxy-leaves must be between 1 and %d", maxLeaves)
	}
	pseudo, err := loadPseudonymizer()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("pseudonym key fingerprint %s (keyed=%t)", pseudo.Fingerprint(), pseudo.Keyed())

	s := DefaultScenario(*wsUID)
	s.AgentUID = *agentUID
	s.Seed = *seed
	g := Generate(s, rand.New(rand.NewSource(s.Seed)))
	c := NewClient(*base, *wsUID, *tokenPath, pseudo)
	ctx := context.Background()
	if _, err := c.CheckKeyBinding(ctx); err != nil {
		fmt.Fprintln(os.Stderr, "seed refused:", err)
		os.Exit(1)
	}
	sum, err := run(ctx, c, g, options{buildGalaxy: *buildGalaxyFlag, leaves: *leaves, seed: *seed})
	report(sum, c)
	if err != nil {
		fmt.Fprintln(os.Stderr, "seed failed:", err)
		os.Exit(1)
	}
}

// report prints what each step did.
func report(sum summary, c *Client) {
	for _, st := range sum.Steps {
		if st.Skipped != "" {
			fmt.Printf("%-20s skipped: %s\n", st.Name, st.Skipped)
			continue
		}
		fmt.Printf("%-20s %d/%d stored, %d failed\n", st.Name, st.Stored, st.Items, st.Failed)
	}
	fmt.Printf("%d writes refused by consent, %d dropped with privacy-api unreachable\n",
		c.Suppressed, c.PrivacyUnavailable)
}
