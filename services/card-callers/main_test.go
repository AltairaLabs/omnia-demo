// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import "testing"

// The canary's first gate decides on the first few conversations, so they must
// already include a disputed payment: a run of lost-card calls first let the
// refund-promising 2.1 pass the 20% gate (r8).
func TestEveryRunOfThreeIncludesADispute(t *testing.T) {
	for start := 0; start < 24; start++ {
		seen := false
		for n := start; n < start+len(scripts); n++ {
			if _, s := assignment(n); s.name == "disputed payment" {
				seen = true
			}
		}
		if !seen {
			t.Fatalf("conversations %d-%d have no disputed payment", start, start+len(scripts)-1)
		}
	}
}

// Every member meets every script, so no script is tied to one caller.
func TestEveryCallerMeetsEveryScript(t *testing.T) {
	pairs := map[string]bool{}
	for n := 0; n < len(callers)*len(scripts); n++ {
		c, s := assignment(n)
		pairs[c.id+"/"+s.name] = true
	}
	if want := len(callers) * len(scripts); len(pairs) != want {
		t.Fatalf("got %d caller/script pairs, want %d", len(pairs), want)
	}
}
