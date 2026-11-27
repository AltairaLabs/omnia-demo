// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type fakeBlob struct {
	info    blobInfo
	body    string
	readErr error
}

type fakeStore struct {
	blobs   []fakeBlob
	listErr error
}

func (f *fakeStore) List(context.Context) ([]blobInfo, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	out := make([]blobInfo, 0, len(f.blobs))
	for _, b := range f.blobs {
		out = append(out, b.info)
	}
	return out, nil
}

func (f *fakeStore) Get(_ context.Context, name string) (io.ReadCloser, error) {
	for _, b := range f.blobs {
		if b.info.Name == name {
			if b.readErr != nil {
				return nil, b.readErr
			}
			return io.NopCloser(strings.NewReader(b.body)), nil
		}
	}
	return nil, errors.New("not found")
}

// recorder is a memory-api stand-in that records every batch it is sent.
func recorder(t *testing.T) (*httptest.Server, *[]batchRequest) {
	t.Helper()
	var got []batchRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var req batchRequest
		if err := json.Unmarshal(body, &req); err != nil {
			t.Errorf("decode: %v", err)
		}
		got = append(got, req)
		w.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(w, `{"accepted":1,"skipped":0,"forgotten":3,"rejected":[]}`)
	}))
	t.Cleanup(srv.Close)
	return srv, &got
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSyncBlobSendsURLSiteModifiedAtAndManifest(t *testing.T) {
	srv, got := recorder(t)
	modified := time.Date(2026, 11, 9, 21, 14, 3, 0, time.FixedZone("x", 3600))
	store := &fakeStore{blobs: []fakeBlob{
		{info: blobInfo{Name: "claims/guide.md", LastModified: modified}, body: "# hi"},
		{info: blobInfo{Name: "photo.png"}, body: "x"},
		{info: blobInfo{Name: "..data/guide.md"}, body: "dup"},
		{info: blobInfo{Name: "notes.txt", LastModified: modified}, body: "n"},
	}}
	cfg := config{memoryURL: srv.URL, workspace: "ws", source: "blob",
		blobAccountURL: "https://acct.blob.core.windows.net/", blobContainer: "larkmere-documents"}
	if err := syncBlob(context.Background(), store, cfg, srv.Client(), quiet()); err != nil {
		t.Fatalf("syncBlob: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("posts = %d, want 1", len(*got))
	}
	req := (*got)[0]
	if req.Source != "blob" || len(req.Documents) != 2 {
		t.Fatalf("source %q documents %d, want blob and 2", req.Source, len(req.Documents))
	}
	d := req.Documents[0]
	if d.URL != "https://acct.blob.core.windows.net/larkmere-documents/claims/guide.md" {
		t.Errorf("url = %s", d.URL)
	}
	if d.Site != "https://acct.blob.core.windows.net/larkmere-documents" {
		t.Errorf("site = %s", d.Site)
	}
	if d.ModifiedAt != "2026-11-09T20:14:03Z" {
		t.Errorf("modified_at = %s, want UTC RFC3339", d.ModifiedAt)
	}
	if !strings.HasPrefix(d.ContentHash, "sha256:") || d.Title != "guide.md" {
		t.Errorf("hash %q title %q", d.ContentHash, d.Title)
	}
	if req.Present == nil || req.Present.Kind != "document" || len(req.Present.Keys) != 2 {
		t.Fatalf("present = %+v, want 2 keys", req.Present)
	}
}

func TestSyncBlobListErrorPostsNothing(t *testing.T) {
	srv, got := recorder(t)
	store := &fakeStore{listErr: errors.New("403 AuthorizationPermissionMismatch")}
	cfg := config{memoryURL: srv.URL, workspace: "ws", source: "blob", blobAccountURL: "https://a", blobContainer: "c"}
	err := syncBlob(context.Background(), store, cfg, srv.Client(), quiet())
	if err == nil {
		t.Fatal("a list failure must fail the run")
	}
	if len(*got) != 0 {
		t.Fatalf("posts = %d, want 0: an empty present would forget everything", len(*got))
	}
}

func TestSyncBlobUnreadableBlobStaysInManifest(t *testing.T) {
	srv, got := recorder(t)
	store := &fakeStore{blobs: []fakeBlob{
		{info: blobInfo{Name: "ok.md"}, body: "ok"},
		{info: blobInfo{Name: "bad.md"}, readErr: errors.New("boom")},
	}}
	cfg := config{memoryURL: srv.URL, workspace: "ws", source: "blob", blobAccountURL: "https://a", blobContainer: "c"}
	if err := syncBlob(context.Background(), store, cfg, srv.Client(), quiet()); err != nil {
		t.Fatalf("syncBlob: %v", err)
	}
	req := (*got)[len(*got)-1]
	if len(req.Documents) != 1 || len(req.Present.Keys) != 2 {
		t.Fatalf("documents %d keys %d, want 1 and 2", len(req.Documents), len(req.Present.Keys))
	}
}

func TestForgetSendsEmptyManifestForNamedSourceOnly(t *testing.T) {
	srv, got := recorder(t)
	cfg := config{memoryURL: srv.URL, workspace: "ws", source: "blob", forgetSource: "wrenmoor-field-procedures"}
	if err := forget(context.Background(), cfg, srv.Client(), quiet()); err != nil {
		t.Fatalf("forget: %v", err)
	}
	if len(*got) != 1 {
		t.Fatalf("posts = %d, want 1", len(*got))
	}
	req := (*got)[0]
	if req.Source != "wrenmoor-field-procedures" || req.WorkspaceID != "ws" {
		t.Errorf("source %q workspace %q", req.Source, req.WorkspaceID)
	}
	if req.Documents == nil || len(req.Documents) != 0 {
		t.Errorf("documents = %v, want an empty array", req.Documents)
	}
	if req.Present == nil || req.Present.Kind != "document" || req.Present.Keys == nil || len(req.Present.Keys) != 0 {
		t.Errorf("present = %+v, want kind document with empty keys", req.Present)
	}
}

func TestLoadConfigBlobAndForget(t *testing.T) {
	for _, name := range []string{"MEMORY_API_URL", "WORKSPACE_ID", "BLOB_ACCOUNT_URL", "BLOB_CONTAINER", "SOURCE_DIR", "FORGET_SOURCE", "MEMORY_SOURCE"} {
		t.Setenv(name, "")
	}
	t.Setenv("MEMORY_API_URL", "http://memory")
	t.Setenv("WORKSPACE_ID", "ws")
	if _, err := loadConfig(); err == nil {
		t.Fatal("blob settings should be required")
	}
	t.Setenv("BLOB_ACCOUNT_URL", "https://a")
	t.Setenv("BLOB_CONTAINER", "c")
	c, err := loadConfig()
	if err != nil || c.source != "blob" {
		t.Fatalf("loadConfig = %+v, %v; want source blob", c, err)
	}
	t.Setenv("BLOB_ACCOUNT_URL", "")
	t.Setenv("BLOB_CONTAINER", "")
	t.Setenv("FORGET_SOURCE", "old")
	if c, err := loadConfig(); err != nil || c.forgetSource != "old" {
		t.Fatalf("forget mode needs no blob settings: %+v, %v", c, err)
	}
}

// The real azblob client against an httptest server speaking List Blobs and
// Get Blob, so the adapter's paging and Last-Modified handling are exercised.
func TestAzblobStoreListsAndReads(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Query().Get("comp") == "list":
			w.Header().Set("Content-Type", "application/xml")
			if r.URL.Query().Get("marker") == "" {
				fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><EnumerationResults ContainerName="c"><Blobs><Blob><Name>a.md</Name><Properties><Last-Modified>Mon, 09 Nov 2026 21:14:03 GMT</Last-Modified></Properties></Blob></Blobs><NextMarker>m2</NextMarker></EnumerationResults>`)
				return
			}
			fmt.Fprint(w, `<?xml version="1.0" encoding="utf-8"?><EnumerationResults ContainerName="c"><Blobs><Blob><Name>b/c.md</Name><Properties><Last-Modified>Tue, 10 Nov 2026 01:00:00 GMT</Last-Modified></Properties></Blob></Blobs><NextMarker/></EnumerationResults>`)
		case r.URL.Path == "/c/a.md":
			_, _ = io.WriteString(w, "body-a")
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	srvURL = srv.URL
	client, err := azblob.NewClientWithNoCredential(srvURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	store := &azblobStore{client: client, container: "c"}
	blobs, err := store.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(blobs) != 2 || blobs[1].Name != "b/c.md" || blobs[0].LastModified.UTC().Format(time.RFC3339) != "2026-11-09T21:14:03Z" {
		t.Fatalf("blobs = %+v", blobs)
	}
	body, err := store.Get(context.Background(), "a.md")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	data, _ := io.ReadAll(body)
	_ = body.Close()
	if string(data) != "body-a" {
		t.Errorf("body = %q", data)
	}
}

// The claims agents' access filter keeps documents whose reference contains
// /member-guidance/. A blob in that folder must therefore be listed (a flat
// listing returns nested names) and carry a URL that contains it, in the
// document and in the manifest, while a top-level blob does not.
func TestSyncBlobNestedFolderURLMatchesTheMemberGuidanceFilter(t *testing.T) {
	srv, got := recorder(t)
	store := &fakeStore{blobs: []fakeBlob{
		{info: blobInfo{Name: "member-guidance/flood-claims.md"}, body: "# flood"},
		{info: blobInfo{Name: "claims-handling.md"}, body: "# rules"},
	}}
	cfg := config{memoryURL: srv.URL, workspace: "ws", source: "blob",
		blobAccountURL: "https://acct.blob.core.windows.net", blobContainer: "larkmere-mutual-documents"}
	if err := syncBlob(context.Background(), store, cfg, srv.Client(), quiet()); err != nil {
		t.Fatalf("syncBlob: %v", err)
	}
	req := (*got)[0]
	const want = "https://acct.blob.core.windows.net/larkmere-mutual-documents/member-guidance/flood-claims.md"
	var matched []string
	for _, d := range req.Documents {
		if strings.Contains(d.URL, "/member-guidance/") {
			matched = append(matched, d.URL)
		}
	}
	if len(matched) != 1 || matched[0] != want {
		t.Errorf("documents matching the filter = %v, want [%s]", matched, want)
	}
	if req.Present == nil || len(req.Present.Keys) != 2 {
		t.Fatalf("present = %+v", req.Present)
	}
	if !strings.Contains(strings.Join(req.Present.Keys, " "), want) {
		t.Errorf("manifest lacks %s: %v", want, req.Present.Keys)
	}
}
