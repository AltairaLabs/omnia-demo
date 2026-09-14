// Package main serves the Larkmere Mutual member portal — the customer-facing
// surface for the synthetic privacy demo.
//
// Why this exists rather than driving the demo from the dashboard Console: the
// Console is an OPERATOR surface. Filming a cardholder typing their Social
// Security number into admin tooling quietly says customers have access to your
// ops console. Splitting the two makes the cut between them the argument — what
// the customer typed in the clear, and what the operator can see afterwards.
//
// It also renders the demo's AI disclosure in the customer-facing surface.
//
// The static site is embedded and served as-is.
//
// This file is FLAG PARSING AND LISTEN ONLY. Everything testable lives in
// server.go, because hack/coverage-exclusions.txt exempts a `main.go` whole —
// keeping logic here would exempt the logic too.
package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	agent := flag.String("agent-url", "http://localhost:8080",
		"base URL of the agent facade's external listener")
	var m member
	flag.StringVar(&m.id, "member-id", "4471-889", "member number asserted to the agent (x-user-id)")
	flag.StringVar(&m.email, "member-email", "ellen.whitcomb@example.com",
		"member email asserted to the agent (x-user-email)")
	flag.StringVar(&m.roles, "member-roles", "member", "roles asserted to the agent (x-user-roles)")
	flag.Parse()

	handler, err := newServer(*agent, m)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("larkmere portal listening on %s (entry %s)", *addr, portalIndex)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}
