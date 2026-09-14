// Command sharepoint-adapter is a demo-only HTTP bridge from an Omnia agent to
// Microsoft SharePoint via the Graph API, and the worked example of a memory
// populator: its `seed` mode syncs a site into institutional memory through
// memory-api's batch ingest contract (see README.md). It is NOT a
// reusable/product connector; source connectors are never product code.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore/policy"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg := loadConfig()

	tokenSource, err := newAzureTokenSource(cfg)
	if err != nil {
		log.Error("create token source", "err", err.Error())
		os.Exit(1)
	}
	client := NewGraphClient(cfg.GraphBaseURL, cfg.SiteID, tokenSource, nil)

	mode := "serve"
	if len(os.Args) > 1 {
		mode = os.Args[1]
	}

	switch mode {
	case "seed":
		seeder := &Seeder{
			src:         client,
			memoryURL:   cfg.MemoryURL,
			workspaceID: cfg.WorkspaceID,
			http:        http.DefaultClient,
			log:         log,
			tokenPath:   cfg.MemoryTokenPath,
		}
		// Say which mode we are in before doing any work. Seeding
		// unauthenticated against a memory-api that requires a token fails on
		// every document with an identical 401, and the run still exits 0 with
		// "seed complete seeded=0" — a green job that indexed nothing.
		log.Info("seeding", "memoryURL", cfg.MemoryURL, "workspace", cfg.WorkspaceID,
			"source", seedSource, "authenticated", cfg.MemoryTokenPath != "")
		n, total, err := seeder.Run(context.Background())
		if err != nil {
			log.Error("seed failed", "seeded", n, "err", err.Error())
			os.Exit(1)
		}
		// A run that found documents and indexed NONE of them is a failure, not
		// a success with a small number in it. Exiting 0 here is what let the
		// corpus sit empty behind a Job reporting Completed: the agent then had
		// no knowledge references, invented document URLs, and the fetch tool
		// took the blame for a seeding problem.
		//
		// Zero of zero is left green deliberately — an empty SharePoint site is
		// a legitimate state, and failing on it would break a first install.
		if total > 0 && n == 0 {
			log.Error("seed indexed nothing", "documents", total,
				"hint", "every ingest failed; check memory-api auth and the error above")
			os.Exit(1)
		}
		// Embed now rather than leaving the corpus searchable only after the
		// re-embed worker's next tick, which is an hour by default. Without
		// this a seed reports success and the agent still finds nothing.
		if n > 0 {
			seeder.DrainEmbeddings(context.Background())
		}
		log.Info("seed complete", "seeded", n, "documents", total)
	default:
		srv := NewServer(client, log)
		addr := ":" + cfg.Port
		log.Info("listening", "addr", addr)
		if err := http.ListenAndServe(addr, srv.Routes()); err != nil {
			log.Error("server exited", "err", err.Error())
			os.Exit(1)
		}
	}
}

// newAzureTokenSource returns a Graph token source backed by an Entra app
// registration's client secret.
func newAzureTokenSource(cfg Config) (TokenSource, error) {
	cred, err := azidentity.NewClientSecretCredential(cfg.TenantID, cfg.ClientID, cfg.ClientSecret, nil)
	if err != nil {
		return nil, err
	}
	return func(ctx context.Context) (string, error) {
		tok, err := cred.GetToken(ctx, policy.TokenRequestOptions{
			Scopes: []string{"https://graph.microsoft.com/.default"},
		})
		if err != nil {
			return "", err
		}
		return tok.Token, nil
	}, nil
}
