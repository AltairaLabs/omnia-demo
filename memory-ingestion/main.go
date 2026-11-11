// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

// Command memory-ingestion is a small, source-only example of an ingestion
// service. It reads text-like objects from S3, or text-like files from a local
// directory (SOURCE_DIR, for example a mounted ConfigMap), and sends documents to
// Omnia's institutional batch-ingest API. It never connects to the memory
// database.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

const (
	batchSize      = 20
	maxObjectBytes = 16 << 20
)

type objectStore interface {
	ListObjectsV2(context.Context, *s3.ListObjectsV2Input, ...func(*s3.Options)) (*s3.ListObjectsV2Output, error)
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
}

type config struct {
	memoryURL, workspace, bucket, prefix, source, tokenPath, dir string
}

type document struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Site        string `json:"site,omitempty"`
	Text        string `json:"text"`
	ModifiedAt  string `json:"modified_at,omitempty"`
	ContentHash string `json:"content_hash"`
}

type manifest struct {
	Kind string   `json:"kind"`
	Keys []string `json:"keys"`
}

// batchResponse is memory-api's 202 body.
type batchResponse struct {
	Accepted  int   `json:"accepted"`
	Skipped   int   `json:"skipped"`
	Forgotten int64 `json:"forgotten"`
	Rejected  []struct {
		Key   string `json:"key"`
		Error string `json:"error"`
	} `json:"rejected"`
	EmbeddingBacklog int `json:"embedding_backlog"`
}

// tally adds up what memory-api reported across a run's batches.
type tally struct {
	accepted, skipped int
	forgotten         int64
	backlog           int
}

type batchRequest struct {
	WorkspaceID string     `json:"workspace_id,omitempty"`
	Source      string     `json:"source"`
	Documents   []document `json:"documents"`
	Present     *manifest  `json:"present,omitempty"`
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		log.Error("invalid configuration", "error", err)
		os.Exit(2)
	}

	ctx := context.Background()
	if cfg.dir != "" {
		if err := syncDir(ctx, cfg, http.DefaultClient, log); err != nil {
			log.Error("sync failed", "error", err)
			os.Exit(1)
		}
		return
	}
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(env("S3_REGION", "us-east-1")))
	if err != nil {
		log.Error("load AWS configuration", "error", err)
		os.Exit(1)
	}
	client := s3.NewFromConfig(awsCfg, func(options *s3.Options) {
		if endpoint := os.Getenv("S3_ENDPOINT"); endpoint != "" {
			options.BaseEndpoint = aws.String(endpoint)
		}
		options.UsePathStyle = strings.EqualFold(os.Getenv("S3_FORCE_PATH_STYLE"), "true")
	})
	if err := sync(ctx, client, cfg, http.DefaultClient, log); err != nil {
		log.Error("sync failed", "error", err)
		os.Exit(1)
	}
}

func loadConfig() (config, error) {
	c := config{
		memoryURL: env("MEMORY_API_URL", ""), workspace: env("WORKSPACE_ID", ""),
		bucket: env("S3_BUCKET", ""), prefix: env("S3_PREFIX", ""),
		source: env("MEMORY_SOURCE", "s3"), tokenPath: env("MEMORY_TOKEN_PATH", ""),
		dir: env("SOURCE_DIR", ""),
	}
	required := map[string]string{"MEMORY_API_URL": c.memoryURL, "WORKSPACE_ID": c.workspace}
	if c.dir == "" {
		required["S3_BUCKET"] = c.bucket
	}
	for name, value := range required {
		if value == "" {
			return config{}, fmt.Errorf("%s is required", name)
		}
	}
	return c, nil
}

func sync(ctx context.Context, store objectStore, cfg config, client *http.Client, log *slog.Logger) error {
	var n tally
	present := &manifest{Kind: "document", Keys: []string{}}
	batch := make([]document, 0, batchSize)
	var continuation *string
	for {
		page, err := store.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket: aws.String(cfg.bucket), Prefix: aws.String(cfg.prefix), ContinuationToken: continuation,
		})
		if err != nil {
			return fmt.Errorf("list S3 objects: %w", err)
		}
		for _, object := range page.Contents {
			key := aws.ToString(object.Key)
			if hidden(strings.TrimPrefix(key, cfg.prefix)) {
				continue // not part of the source; see hidden
			}
			url := "s3://" + cfg.bucket + "/" + key
			present.Keys = append(present.Keys, url)
			doc, err := read(ctx, store, cfg.bucket, key, object.LastModified)
			if err != nil {
				log.Warn("skip object", "key", key, "error", err)
				continue
			}
			batch = append(batch, doc)
			if len(batch) == batchSize {
				if err := post(ctx, client, cfg, batch, nil, &n); err != nil {
					return err
				}
				batch = batch[:0]
			}
		}
		if !aws.ToBool(page.IsTruncated) || page.NextContinuationToken == nil {
			break
		}
		continuation = page.NextContinuationToken
	}
	if err := post(ctx, client, cfg, batch, present, &n); err != nil {
		return err
	}
	log.Info("sync complete", "bucket", cfg.bucket, "prefix", cfg.prefix, "documents", len(present.Keys),
		"accepted", n.accepted, "skipped", n.skipped, "forgotten", n.forgotten, "embedding_backlog", n.backlog)
	return nil
}

// syncDir ingests every text-like file directly in cfg.dir, then sends the
// manifest so memory-api forgets documents no longer present. Entries whose
// names start with "." are skipped: a mounted ConfigMap keeps its real files
// under "..data" and exposes them as symlinks, which os.ReadFile follows.
func syncDir(ctx context.Context, cfg config, client *http.Client, log *slog.Logger) error {
	entries, err := os.ReadDir(cfg.dir)
	if err != nil {
		return fmt.Errorf("read source directory: %w", err)
	}
	var n tally
	present := &manifest{Kind: "document", Keys: []string{}}
	batch := make([]document, 0, batchSize)
	for _, entry := range entries {
		name := entry.Name()
		if strings.HasPrefix(name, ".") || entry.IsDir() || !textLike(name) {
			continue
		}
		// In the manifest before it is read, as the S3 path does: a file that
		// fails to read this time is skipped, but memory-api keeps the copy it
		// already holds rather than forgetting it.
		url := "file://" + cfg.source + "/" + name
		present.Keys = append(present.Keys, url)
		body, err := os.ReadFile(filepath.Join(cfg.dir, name))
		if err != nil {
			log.Warn("skip file", "name", name, "error", err)
			continue
		}
		if len(body) > maxObjectBytes {
			log.Warn("skip file", "name", name, "error", "exceeds size limit")
			continue
		}
		hash := sha256.Sum256(body)
		batch = append(batch, document{Title: name, URL: url, Site: "file://" + cfg.source, Text: string(body), ContentHash: "sha256:" + hex.EncodeToString(hash[:])})
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
	log.Info("sync complete", "dir", cfg.dir, "documents", len(present.Keys),
		"accepted", n.accepted, "skipped", n.skipped, "forgotten", n.forgotten, "embedding_backlog", n.backlog)
	return nil
}

func read(ctx context.Context, store objectStore, bucket, key string, modified *time.Time) (document, error) {
	if !textLike(key) {
		return document{}, fmt.Errorf("unsupported file type")
	}
	out, err := store.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(key)})
	if err != nil {
		return document{}, fmt.Errorf("get object: %w", err)
	}
	defer func() { _ = out.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(out.Body, maxObjectBytes+1))
	if err != nil {
		return document{}, fmt.Errorf("read object: %w", err)
	}
	if len(body) > maxObjectBytes {
		return document{}, fmt.Errorf("object exceeds %d-byte limit", maxObjectBytes)
	}
	hash := sha256.Sum256(body)
	doc := document{Title: filepath.Base(key), URL: "s3://" + bucket + "/" + key, Site: "s3://" + bucket, Text: string(body), ContentHash: "sha256:" + hex.EncodeToString(hash[:])}
	if modified != nil {
		doc.ModifiedAt = modified.UTC().Format(time.RFC3339)
	}
	return doc, nil
}

// post sends one batch and adds memory-api's counts to n. A network error, a
// 429 or a 5xx is retried twice with backoff; any other non-2xx is the
// request's fault and fails at once, with memory-api's error text. A document
// memory-api rejects fails the run too, so a bad document is never silently
// missing from memory.
func post(ctx context.Context, client *http.Client, cfg config, docs []document, present *manifest, n *tally) error {
	payload, err := json.Marshal(batchRequest{WorkspaceID: cfg.workspace, Source: cfg.source, Documents: docs, Present: present})
	if err != nil {
		return fmt.Errorf("encode batch: %w", err)
	}
	endpoint := strings.TrimRight(cfg.memoryURL, "/") + "/api/v1/institutional/ingest/batch"
	for attempt := 1; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(string(payload)))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.tokenPath != "" {
			// Read every time, so a rotated projected token is picked up.
			token, readErr := os.ReadFile(cfg.tokenPath)
			if readErr != nil {
				return fmt.Errorf("read memory token: %w", readErr)
			}
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
		}
		resp, err := client.Do(req)
		var retry error
		switch {
		case err != nil:
			retry = fmt.Errorf("post batch: %w", err)
		case resp.StatusCode >= 200 && resp.StatusCode < 300:
			defer func() { _ = resp.Body.Close() }()
			var result batchResponse
			if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
				return fmt.Errorf("decode batch response: %w", err)
			}
			if len(result.Rejected) > 0 {
				return fmt.Errorf("memory-api rejected %d documents (first: %s: %s)",
					len(result.Rejected), result.Rejected[0].Key, result.Rejected[0].Error)
			}
			n.accepted += result.Accepted
			n.skipped += result.Skipped
			n.forgotten += result.Forgotten
			n.backlog = result.EmbeddingBacklog
			return nil
		default:
			message, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
			_ = resp.Body.Close()
			failure := fmt.Errorf("memory-api returned %s: %s", resp.Status, strings.TrimSpace(string(message)))
			if resp.StatusCode != http.StatusTooManyRequests && resp.StatusCode < 500 {
				return failure
			}
			retry = failure
		}
		if attempt == 3 {
			return retry
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
}

// hidden reports whether any segment of a key's path starts with ".". Such
// objects are skipped, and left out of the manifest: a directory uploaded from
// a mounted ConfigMap carries every file again under ..data/ and a timestamped
// ..<date>/ directory, and ingesting those would store each document twice
// or more.
func hidden(key string) bool {
	for _, segment := range strings.Split(key, "/") {
		if strings.HasPrefix(segment, ".") {
			return true
		}
	}
	return false
}

func textLike(key string) bool {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".txt", ".md", ".markdown", ".csv", ".json", ".yaml", ".yml", ".log":
		return true
	default:
		return false
	}
}

func env(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
