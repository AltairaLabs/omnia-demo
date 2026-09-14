// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

// Command memory-ingestion is a small, source-only example of an ingestion
// service. It reads text-like objects from S3 and sends documents to Omnia's
// institutional batch-ingest API. It never connects to the memory database.
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
	memoryURL, workspace, bucket, prefix, source, tokenPath string
}

type document struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	Text        string `json:"text"`
	ModifiedAt  string `json:"modified_at,omitempty"`
	ContentHash string `json:"content_hash"`
}

type manifest struct {
	Kind string   `json:"kind"`
	Keys []string `json:"keys"`
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
	}
	for name, value := range map[string]string{"MEMORY_API_URL": c.memoryURL, "WORKSPACE_ID": c.workspace, "S3_BUCKET": c.bucket} {
		if value == "" {
			return config{}, fmt.Errorf("%s is required", name)
		}
	}
	return c, nil
}

func sync(ctx context.Context, store objectStore, cfg config, client *http.Client, log *slog.Logger) error {
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
			url := "s3://" + cfg.bucket + "/" + key
			present.Keys = append(present.Keys, url)
			doc, err := read(ctx, store, cfg.bucket, key, object.LastModified)
			if err != nil {
				log.Warn("skip object", "key", key, "error", err)
				continue
			}
			batch = append(batch, doc)
			if len(batch) == batchSize {
				if err := post(ctx, client, cfg, batch, nil); err != nil {
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
	if err := post(ctx, client, cfg, batch, present); err != nil {
		return err
	}
	log.Info("sync complete", "bucket", cfg.bucket, "prefix", cfg.prefix, "documents", len(present.Keys))
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
	doc := document{Title: filepath.Base(key), URL: "s3://" + bucket + "/" + key, Text: string(body), ContentHash: "sha256:" + hex.EncodeToString(hash[:])}
	if modified != nil {
		doc.ModifiedAt = modified.UTC().Format(time.RFC3339)
	}
	return doc, nil
}

func post(ctx context.Context, client *http.Client, cfg config, docs []document, present *manifest) error {
	payload, err := json.Marshal(batchRequest{WorkspaceID: cfg.workspace, Source: cfg.source, Documents: docs, Present: present})
	if err != nil {
		return fmt.Errorf("encode batch: %w", err)
	}
	for attempt := 1; attempt <= 3; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(cfg.memoryURL, "/")+"/api/v1/institutional/ingest/batch", strings.NewReader(string(payload)))
		if err != nil {
			return fmt.Errorf("create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.tokenPath != "" {
			token, readErr := os.ReadFile(cfg.tokenPath)
			if readErr != nil {
				return fmt.Errorf("read memory token: %w", readErr)
			}
			req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
		}
		resp, err := client.Do(req)
		if err == nil && resp.StatusCode >= 200 && resp.StatusCode < 300 {
			_ = resp.Body.Close()
			return nil
		}
		if resp != nil {
			_ = resp.Body.Close()
		}
		if attempt == 3 {
			if err != nil {
				return fmt.Errorf("post batch: %w", err)
			}
			return fmt.Errorf("post batch: HTTP %s", resp.Status)
		}
		time.Sleep(time.Duration(attempt) * 2 * time.Second)
	}
	return nil
}

func textLike(key string) bool {
	switch strings.ToLower(filepath.Ext(key)) {
	case ".txt", ".md", ".csv", ".json", ".yaml", ".yml", ".log":
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
