// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/AltairaLabs/omnia-demo/demos/memory-seeder/identity"
	"golang.org/x/time/rate"
)

// agentListing serves one page of agent memories and records deletes.
func agentListing(t *testing.T, memories []map[string]any) (*Client, *[]string) {
	t.Helper()
	var mu sync.Mutex
	deleted := []string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			writeJSON(w, http.StatusOK, map[string]any{"memories": memories})
		case http.MethodDelete:
			mu.Lock()
			deleted = append(deleted, strings.TrimPrefix(r.URL.Path, pathAgentMemories+"/"))
			mu.Unlock()
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	t.Cleanup(srv.Close)
	c := NewClient(srv.URL, testWorkspace, filepath.Join(t.TempDir(), "absent"), identity.New(""))
	c.limiter = rate.NewLimiter(rate.Inf, 1)
	return c, &deleted
}

func mem(id, content, kind, key string) map[string]any {
	md := map[string]any{}
	if kind != "" {
		md["about_kind"], md["about_key"] = kind, key
	}
	return map[string]any{"id": id, "content": content, "metadata": md}
}

// A dropped corpus id and a key-less copy of corpus text are forgotten; a
// current corpus memory and one the agent learned itself are kept.
func TestPruneAgentForgetsOnlyStaleReplays(t *testing.T) {
	c, deleted := agentListing(t, []map[string]any{
		mem("m1", "kept", fixtureKind, "a1"),
		mem("m2", "dropped", fixtureKind, "gone"),
		mem("m3", "old copy", "", ""),
		mem("m4", "learned by the agent", "", ""),
	})
	n, err := c.PruneAgent(context.Background(), "agent-uid",
		map[string]bool{"a1": true}, map[string]bool{"kept": true, "old copy": true})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(*deleted)
	if n != 2 || strings.Join(*deleted, ",") != "m2,m3" {
		t.Fatalf("pruned %d: %v", n, *deleted)
	}
}

// If the listing echoes no about keys at all, every replayed memory looks
// key-less: PruneAgent must refuse rather than delete the tier it just wrote.
func TestPruneAgentRefusesWhenListingHasNoKeys(t *testing.T) {
	c, deleted := agentListing(t, []map[string]any{
		mem("m1", "kept", "", ""),
		mem("m2", "also kept", "", ""),
	})
	_, err := c.PruneAgent(context.Background(), "agent-uid",
		map[string]bool{"a1": true, "a2": true}, map[string]bool{"kept": true, "also kept": true})
	if err == nil || len(*deleted) != 0 {
		t.Fatalf("err %v, deleted %v", err, *deleted)
	}
}
