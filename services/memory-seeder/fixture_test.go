// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"
	"time"
)

// The embedded Larkmere corpus loads, every memory names a tier the seeder
// writes, and every graph edge ends at a node the graph holds.
func TestLarkmereGalaxyFixtureLoads(t *testing.T) {
	f, err := loadFixture("larkmere-galaxy")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.memories) == 0 || len(f.graph.Items) == 0 || len(f.graph.Edges) == 0 {
		t.Fatalf("empty fixture: %d memories, %d items, %d edges", len(f.memories), len(f.graph.Items), len(f.graph.Edges))
	}
	ids := map[string]bool{}
	for _, m := range f.memories {
		switch m.Tier {
		case "user":
			if m.Member == "" || m.Agent != "" {
				t.Fatalf("user memory %s: member %q agent %q", m.ID, m.Member, m.Agent)
			}
		case "user_for_agent":
			if m.Member == "" || m.Agent == "" {
				t.Fatalf("user_for_agent memory %s needs a member and an agent", m.ID)
			}
		case "agent":
			if m.Agent == "" || m.Member != "" {
				t.Fatalf("agent memory %s: member %q agent %q", m.ID, m.Member, m.Agent)
			}
		default:
			t.Fatalf("memory %s has tier %q", m.ID, m.Tier)
		}
		if ids[m.ID] {
			t.Fatalf("duplicate id %s: re-seeding would supersede one with the other", m.ID)
		}
		ids[m.ID] = true
	}
	nodes := map[string]string{}
	for _, it := range f.graph.Items {
		nodes[it.Key] = it.Kind
	}
	for _, e := range f.graph.Edges {
		if nodes[e.From] != e.FromKind || nodes[e.To] != e.ToKind {
			t.Fatalf("edge %s -> %s (%s) names a node the graph does not hold", e.From, e.To, e.Type)
		}
	}
}

// Agent memories get a stable expiry inside the window, and not all the same.
func TestAgentExpirySpreadsAndIsStable(t *testing.T) {
	now := time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC)
	seen := map[time.Time]bool{}
	for _, id := range []string{"agent/a/0-0", "agent/a/0-1", "agent/b/1-0", "agent/c/2-3"} {
		e := agentExpiry(id, now)
		if e.Before(now.Add(agentExpiryMin)) || !e.Before(now.Add(agentExpiryMax)) {
			t.Fatalf("%s expires %s, outside the window", id, e)
		}
		if !agentExpiry(id, now).Equal(e) {
			t.Fatalf("%s: expiry not stable", id)
		}
		seen[e] = true
	}
	if len(seen) < 2 {
		t.Fatal("every agent memory got the same expiry")
	}
}

// Members are granted exactly the categories that need a grant, and only the
// ones their own memories use.
func TestConsentGrantsOnlyWhatIsNeeded(t *testing.T) {
	f := fixture{memories: []fixtureMemory{
		{Member: "m1", Category: "health"}, {Member: "m1", Category: "history"},
		{Member: "m1", Category: "location"}, {Member: "m2", Category: "context"},
		{Agent: "claims-triage", Category: "identity"},
	}}
	g := f.consentGrants()
	if strings.Join(g["m1"], ",") != "memory:health,memory:location" {
		t.Fatalf("m1 grants %v", g["m1"])
	}
	if _, ok := g["m2"]; ok {
		t.Fatalf("m2 needs no grant, got %v", g["m2"])
	}
}
