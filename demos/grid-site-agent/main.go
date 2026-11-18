// Command grid-site-agent is Wrenmoor Energy's on-site grid-operations
// assistant: a PromptKit pack whose tools (SCADA reads, work orders) run here,
// inside the site's operational network, served to Omnia over one outbound
// dial-home connection. The site accepts no inbound connections; the agent
// dials home to its AgentRuntime's relay with the key Omnia issued, and Omnia
// sends it conversations over that connection.
//
// It is the omnia-dialhome entrypoint (github.com/AltairaLabs/omnia-dialhome,
// cmd/omnia-dialhome) plus the site's tools: that command can run a pack's
// client tools but not tools of its own, so a site with local tools builds its
// own small main on the library, as this one does.
//
//	OMNIA_DIAL_HOME_URL     the relay's ws(s)://…/v1/channel URL (required)
//	OMNIA_DIAL_HOME_KEY     the agent's dial-home key (required)
//	OMNIA_DIAL_HOME_CONFIG  baked provider configuration, JSON (optional)
//	SITE_AGENT_PACK         the compiled pack (or -pack)
//	SITE_AGENT_PROMPT       the pack prompt to run (or -prompt)
//	SITE_AGENT_MODEL        "script" (default): the scripted mock, no model and
//	                        no key; "omnia": the Provider Omnia serves over the
//	                        channel (or -model)
//	PORT                    when set, serve /healthz and /readyz on it
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	dialhome "github.com/AltairaLabs/omnia-dialhome"
	"github.com/AltairaLabs/omnia-dialhome/promptkit"
	"github.com/AltairaLabs/omnia-dialhome/providerspec"
	"github.com/AltairaLabs/omnia-dialhome/server"
	"github.com/go-logr/logr"
)

const (
	defaultPack   = "/pack/grid-site-agent.pack.json"
	defaultPrompt = "grid_operations"
	modelScript   = "script"
	modelOmnia    = "omnia"
)

// Exit codes.
const (
	exitOK = iota
	exitUsage
	exitConfig
	exitServe
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv))
}

type options struct {
	pack, prompt, model, port string
}

func parse(args []string, getenv func(string) string) (options, error) {
	fs := flag.NewFlagSet("grid-site-agent", flag.ContinueOnError)
	pack := fs.String("pack", orDefault(getenv("SITE_AGENT_PACK"), defaultPack), "compiled PromptPack (.pack.json)")
	prompt := fs.String("prompt", orDefault(getenv("SITE_AGENT_PROMPT"), defaultPrompt), "pack prompt to run")
	model := fs.String("model", orDefault(getenv("SITE_AGENT_MODEL"), modelScript), "script or omnia")
	if err := fs.Parse(args); err != nil {
		return options{}, err
	}
	if *model != modelScript && *model != modelOmnia {
		return options{}, fmt.Errorf("-model must be %q or %q, not %q", modelScript, modelOmnia, *model)
	}
	return options{pack: *pack, prompt: *prompt, model: *model, port: getenv("PORT")}, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func run(args []string, getenv func(string) string) int {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return serve(ctx, args, getenv)
}

// serve runs the site agent until ctx ends.
func serve(ctx context.Context, args []string, getenv func(string) string) int {
	log := logr.FromSlogHandler(slog.NewJSONHandler(os.Stderr, nil))
	opts, err := parse(args, getenv)
	if err != nil {
		log.Error(err, "usage")
		return exitUsage
	}
	cfg, ok, err := dialhome.ConfigFromEnv(getenv)
	if err != nil || !ok {
		log.Error(err, "dial-home is not configured: set OMNIA_DIAL_HOME_URL and OMNIA_DIAL_HOME_KEY")
		return exitConfig
	}
	cfg.Log = log

	ch, err := dialhome.Dial(ctx, cfg)
	if err != nil {
		log.Error(err, "dial-home failed")
		return exitConfig
	}
	defer func() { _ = ch.Close() }()

	provider := scriptProvider
	if opts.model == modelOmnia {
		baked, _, err := dialhome.BakedConfigFromEnv(getenv)
		if err != nil {
			log.Error(err, "baked config ignored")
		}
		provider = servedProvider(dialhome.NewConfigSource(ch, baked), getenv, log)
	}
	h, err := newHandler(opts, newSite(), provider, log)
	if err != nil {
		log.Error(err, "runtime not built")
		return exitConfig
	}
	defer func() { _ = h.Close() }()

	if opts.port != "" {
		go serveProbes(ctx, net.JoinHostPort("", opts.port), ch, log)
	}
	go func() {
		<-ctx.Done()
		_ = ch.Close()
	}()
	log.Info("serving over dial-home", "pack", opts.pack, "prompt", opts.prompt, "model", opts.model)
	if err := server.Serve(ch, h, server.WithLogger(log)); err != nil && !errors.Is(err, net.ErrClosed) {
		log.Error(err, "serve failed")
		return exitServe
	}
	return exitOK
}

// newHandler is the pack served with the site's tools registered on every
// conversation.
func newHandler(opts options, s *site, provider promptkit.ProviderFunc, log logr.Logger) (*promptkit.Handler, error) {
	return promptkit.New(promptkit.Config{
		PackPath:   opts.pack,
		PromptName: opts.prompt,
		Provider:   provider,
		Configure:  s.register,
		Log:        log,
	})
}

func scriptProvider(context.Context) (providers.Provider, error) {
	return newScriptedProvider(), nil
}

// servedProvider builds each conversation's provider from the agent's default
// Provider, as the channel (or the baked config) serves it; with neither,
// PromptKit chooses from the pack and the environment. As cmd/omnia-dialhome
// does.
func servedProvider(src *dialhome.ConfigSource, getenv func(string) string, log logr.Logger) promptkit.ProviderFunc {
	return func(ctx context.Context) (providers.Provider, error) {
		cfg, origin, err := src.Current(ctx)
		if errors.Is(err, dialhome.ErrNoConfig) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		p, ok := cfg.Default()
		if !ok {
			log.Info("no default provider served; PromptKit chooses", "origin", string(origin))
			return nil, nil
		}
		prov, err := providerspec.New(ctx, p, getenv)
		if err != nil {
			return nil, fmt.Errorf("provider %s/%s from %s config: %w", p.Group, p.Name, origin, err)
		}
		return prov, nil
	}
}

// serveProbes answers /healthz while the process runs and /readyz while the
// channel to Omnia is up. It is for the kubelet on the same node; nothing off
// the site needs to reach it.
func serveProbes(ctx context.Context, addr string, ch *dialhome.Client, log logr.Logger) {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		if ch.Connected() {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		<-ctx.Done()
		_ = srv.Close()
	}()
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Error(err, "probe server failed")
	}
}
