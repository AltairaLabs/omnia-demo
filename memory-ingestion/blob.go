// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

// blobInfo is what a container listing gives for one blob.
type blobInfo struct {
	Name         string
	LastModified time.Time
}

// blobStore is the seam over Azure Blob: tests run without network.
type blobStore interface {
	List(ctx context.Context) ([]blobInfo, error)
	Get(ctx context.Context, name string) (io.ReadCloser, error)
}

type azblobStore struct {
	client    *azblob.Client
	container string
}

// newBlobStore authenticates with Entra workload identity when the AKS webhook
// has injected a federated token file, and otherwise with the default
// credential chain (for example `az login` on a laptop). There is no account
// key, SAS or connection-string path.
func newBlobStore(cfg config) (*azblobStore, error) {
	var cred azcore.TokenCredential
	var err error
	if os.Getenv("AZURE_FEDERATED_TOKEN_FILE") != "" {
		cred, err = azidentity.NewWorkloadIdentityCredential(nil)
	} else {
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	}
	if err != nil {
		return nil, fmt.Errorf("azure credential: %w", err)
	}
	client, err := azblob.NewClient(cfg.blobAccountURL, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("blob client: %w", err)
	}
	return &azblobStore{client: client, container: cfg.blobContainer}, nil
}

func (s *azblobStore) List(ctx context.Context) ([]blobInfo, error) {
	var out []blobInfo
	pager := s.client.NewListBlobsFlatPager(s.container, nil)
	for pager.More() {
		page, err := pager.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Segment.BlobItems {
			if item == nil || item.Name == nil {
				continue
			}
			info := blobInfo{Name: *item.Name}
			if item.Properties != nil && item.Properties.LastModified != nil {
				info.LastModified = *item.Properties.LastModified
			}
			out = append(out, info)
		}
	}
	return out, nil
}

func (s *azblobStore) Get(ctx context.Context, name string) (io.ReadCloser, error) {
	resp, err := s.client.DownloadStream(ctx, s.container, name, nil)
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

// syncBlob ingests every text-like blob in the container, then sends the
// manifest so memory-api forgets documents no longer present. The whole
// listing happens before anything is posted: a listing or credential failure
// stops the run with nothing sent, so an empty or partial manifest can never
// make memory-api forget documents that are still in the container.
func syncBlob(ctx context.Context, store blobStore, cfg config, client *http.Client, log *slog.Logger) error {
	blobs, err := store.List(ctx)
	if err != nil {
		return fmt.Errorf("list blobs: %w", err)
	}
	site := strings.TrimRight(cfg.blobAccountURL, "/") + "/" + cfg.blobContainer
	var n tally
	present := &manifest{Kind: "document", Keys: []string{}}
	batch := make([]document, 0, batchSize)
	for _, b := range blobs {
		if hidden(b.Name) || !textLike(b.Name) {
			continue // not part of the source; see hidden
		}
		// In the manifest before it is read, as syncDir does: a blob that fails
		// to read this time is skipped, but memory-api keeps the copy it already
		// holds rather than forgetting it.
		url := site + "/" + b.Name
		present.Keys = append(present.Keys, url)
		body, err := readBlob(ctx, store, b.Name)
		if err != nil {
			log.Warn("skip blob", "name", b.Name, "error", err)
			continue
		}
		hash := sha256.Sum256(body)
		doc := document{Title: path.Base(b.Name), URL: url, Site: site, Text: string(body), ContentHash: "sha256:" + hex.EncodeToString(hash[:])}
		if !b.LastModified.IsZero() {
			doc.ModifiedAt = b.LastModified.UTC().Format(time.RFC3339)
		}
		batch = append(batch, doc)
		if len(batch) == batchSize {
			if err := post(ctx, client, cfg, batch, nil, &n); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	if err := post(ctx, client, cfg, batch, present, &n); err != nil {
		return err
	}
	log.Info("sync complete", "container", site, "documents", len(present.Keys),
		"accepted", n.accepted, "skipped", n.skipped, "forgotten", n.forgotten, "embedding_backlog", n.backlog)
	return nil
}

func readBlob(ctx context.Context, store blobStore, name string) ([]byte, error) {
	rc, err := store.Get(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get blob: %w", err)
	}
	defer func() { _ = rc.Close() }()
	body, err := io.ReadAll(io.LimitReader(rc, maxObjectBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read blob: %w", err)
	}
	if len(body) > maxObjectBytes {
		return nil, fmt.Errorf("blob exceeds %d-byte limit", maxObjectBytes)
	}
	return body, nil
}
