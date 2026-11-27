package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"syscall"
)

// defaultAddr is the address the kit's test tools
// (demos/personal-agent/arena/test/tools) post to.
const defaultAddr = "127.0.0.1:18640"

// Usage:
//
//	arenafake [-addr host:port] [command args...]
//
// With a command (for example `promptarena run --config ...`), it serves
// while the command runs and exits with the command's status. Without one,
// it serves until interrupted.
func main() {
	addr := flag.String("addr", defaultAddr, "address to serve the tools on")
	quiet := flag.Bool("quiet", false, "do not log tool calls")
	flag.Parse()

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		log.Fatalf("arenafake: %v", err)
	}
	f := newFake()
	h := f.handler()
	if !*quiet {
		h = logged(h)
	}
	srv := &http.Server{Handler: h}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("arenafake: %v", err)
		}
	}()
	log.Printf("arenafake: serving the personal-agent tools on http://%s/tools/", *addr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if flag.NArg() == 0 {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
		return
	}
	cmd := exec.CommandContext(ctx, flag.Arg(0), flag.Args()[1:]...) //nolint:gosec // the operator's own command
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	err = cmd.Run()
	_ = srv.Shutdown(context.Background())
	var exit *exec.ExitError
	switch {
	case errors.As(err, &exit):
		os.Exit(exit.ExitCode())
	case err != nil:
		fmt.Fprintf(os.Stderr, "arenafake: %v\n", err)
		os.Exit(1)
	}
}

func logged(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("arenafake: %s %s", r.Method, r.URL.Path)
		h.ServeHTTP(w, r)
	})
}
