// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"context"
	"log/slog"
	"net/http"
)

// forget removes everything memory-api holds for cfg.forgetSource by posting
// one batch with no documents and an empty manifest: every document of that
// source is absent, so every one is forgotten. It touches only the named
// source, never cfg.source.
func forget(ctx context.Context, cfg config, client *http.Client, log *slog.Logger) error {
	log.Warn("forgetting every document of a source", "source", cfg.forgetSource, "workspace", cfg.workspace)
	cfg.source = cfg.forgetSource
	var n tally
	if err := post(ctx, client, cfg, []document{}, &manifest{Kind: "document", Keys: []string{}}, &n); err != nil {
		return err
	}
	log.Info("forget complete", "source", cfg.forgetSource, "forgotten", n.forgotten)
	return nil
}
