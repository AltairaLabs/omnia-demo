// Package main serves the demo companies' customer-facing websites: one
// binary, one image, and a --site flag naming the brand.
//
//	larkmere-mutual   Larkmere Mutual's public site and member portal, the
//	                  customer-facing surface for the synthetic privacy demo.
//	wrenmoor-energy   Wrenmoor Energy's public site, its customer portal with
//	                  the Wren assistant, and its staff intranet.
//	halvern-partners  Halvern Partners' public site, its Practice Desk (the
//	                  internal view of client engagements and agents) and its
//	                  design-system reference.
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

	"github.com/AltairaLabs/omnia-demo/demos/demoidp"
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
	issuerURL := flag.String("issuer-url", "",
		"sign a token for the member on each assistant connection, as this issuer (its JWKS at "+demoidp.JWKSPath+
			"); empty: assert the member in headers only, and the conversation has no verified user")
	var eu endUserPlane
	flag.StringVar(&eu.url, "enduser-url", "",
		"base URL of Omnia's management-api, whose end-user plane serves the member's My data page at "+myDataPrefix+
			"; needs --issuer-url, --enduser-workspace and --enduser-audience; empty: My data is off")
	flag.StringVar(&eu.workspace, "enduser-workspace", "", "the member's Omnia Workspace")
	flag.StringVar(&eu.audience, "enduser-audience", "",
		"the aud of the member's end-user token: the Workspace's spec.endUserAuth.oidc.audience, never an agent's")
	flag.Parse()

	b, err := lookupBrand(*name, m)
	if err != nil {
		log.Fatalf("site: %v", err)
	}
	var iss *demoidp.Issuer
	if *issuerURL != "" {
		if iss, err = demoidp.New(*issuerURL); err != nil {
			log.Fatalf("issuer: %v", err)
		}
	}
	handler, err := newServer(b, *agent, *backend, iss, eu)
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
