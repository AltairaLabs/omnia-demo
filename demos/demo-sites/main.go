// Package main serves the demo companies' customer-facing websites: one
// binary, one image, and a --site flag naming the brand.
//
//	larkmere-mutual   Larkmere Mutual's public site and member portal, the
//	                  customer-facing surface for the synthetic privacy demo.
//	wrenmoor-energy   Wrenmoor Energy's public site, its customer portal with
//	                  the Wren assistant, and its staff intranet.
//
// Why a site at all rather than driving a demo from the dashboard Console: the
// Console is an OPERATOR surface. Filming a customer typing into admin tooling
// quietly says customers have access to your ops console. Splitting the two
// makes the cut between them the argument — what the customer did, and what
// the operator can see afterwards.
//
// The static sites are embedded and served as-is; the only dynamic part is the
// assistant proxy, which asserts the signed-in customer's identity to the
// agent (server.go).
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
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	name := flag.String("site", "larkmere-mutual", "which site to serve: "+strings.Join(brandNames(), ", "))
	agent := flag.String("agent-url", "http://localhost:8080",
		"base URL of the agent facade's external listener")
	backend := flag.String("backend-url", "",
		"base URL of the account back end for the assistant's consent and receipt routes; empty: those routes are off")
	var m member
	flag.StringVar(&m.id, "member-id", "", "customer id asserted to the agent (x-user-id); empty: the site's default")
	flag.StringVar(&m.email, "member-email", "", "customer email asserted to the agent (x-user-email); empty: the site's default")
	flag.StringVar(&m.roles, "member-roles", "", "roles asserted to the agent (x-user-roles); empty: the site's default")
	flag.Parse()

	b, err := lookupBrand(*name, m)
	if err != nil {
		log.Fatalf("site: %v", err)
	}
	handler, err := newServer(b, *agent, *backend)
	if err != nil {
		log.Fatalf("server: %v", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("demo-sites: serving %s on %s (entry %s, agent %s)", b.dir, *addr, b.index, *agent)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}
