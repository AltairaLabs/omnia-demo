// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSyncDirPostsTextFilesAndManifest(t *testing.T) {
	dir := t.TempDir()
	for name, body := range map[string]string{"isolation.md": "isolate first", "notes.txt": "check twice", "photo.png": "x"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	// A mounted ConfigMap keeps its files under ..data; it must be skipped.
	if err := os.Mkdir(filepath.Join(dir, "..data"), 0o700); err != nil {
		t.Fatal(err)
	}

	var got []batchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/institutional/ingest/batch" {
			t.Errorf("path = %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req batchRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode: %v", err)
		}
		got = append(got, req)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"accepted":2,"skipped":0,"forgotten":0,"rejected":[]}`)
	}))
	defer srv.Close()

	cfg := config{memoryURL: srv.URL, workspace: "ws", source: "fixtures", dir: dir}
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := syncDir(context.Background(), cfg, srv.Client(), log); err != nil {
		t.Fatalf("syncDir: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("posts = %d, want 1", len(got))
	}
	if n := len(got[0].Documents); n != 2 {
		t.Fatalf("documents = %d, want 2 (png and ..data skipped)", n)
	}
	if got[0].Present == nil || len(got[0].Present.Keys) != 2 {
		t.Fatalf("manifest = %+v, want 2 keys", got[0].Present)
	}
	if got[0].Documents[0].URL != "file://fixtures/isolation.md" {
		t.Errorf("url = %s", got[0].Documents[0].URL)
	}
}

func TestPostFailsOnRejectionAndOnAClientError(t *testing.T) {
	status, body := http.StatusAccepted, `{"accepted":0,"rejected":[{"key":"file://x/a.md","error":"url owned by another source"}]}`
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(status)
		_, _ = io.WriteString(w, body)
	}))
	defer srv.Close()
	cfg := config{memoryURL: srv.URL, workspace: "ws", source: "x"}
	var n tally
	if err := post(context.Background(), srv.Client(), cfg, []document{{URL: "file://x/a.md"}}, nil, &n); err == nil {
		t.Fatal("a rejected document should fail the run")
	}
	// A 403 is not retried: running again will not fix it.
	status, body, calls = http.StatusForbidden, "enterprise_required", 0
	if err := post(context.Background(), srv.Client(), cfg, nil, nil, &n); err == nil || calls != 1 {
		t.Fatalf("403: err %v after %d calls, want an error after 1", err, calls)
	}
}

func TestLoadConfigDirNeedsNoBucket(t *testing.T) {
	t.Setenv("MEMORY_API_URL", "http://memory")
	t.Setenv("WORKSPACE_ID", "ws")
	t.Setenv("S3_BUCKET", "")
	t.Setenv("SOURCE_DIR", "/fixtures")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("loadConfig with SOURCE_DIR: %v", err)
	}
}
