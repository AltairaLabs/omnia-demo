package main

import (
	"errors"
	"flag"
	"log"
	"net/http"
	"net/url"
	"time"
)

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	accountAPI := flag.String("account-api", "", `serve this Company's account routes: "", "larkmere" or "wrenmoor"`)
	oboIssuer := flag.String("obo-issuer", "", "the issuer of the exchanged on-behalf-of tokens the account routes accept (https)")
	oboAudience := flag.String("obo-audience", "", "the audience those tokens must carry: this API, https://<domain>/api")
	flag.Parse()

	acct, err := accountConfigFrom(*accountAPI, *oboIssuer, *oboAudience)
	if err != nil {
		log.Fatalf("account routes: %v", err)
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           newServerWith(acct),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("income-verify stub listening on %s", *addr)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatalf("server: %v", err)
	}
}

// accountConfigFrom builds the account routes' configuration from the
// flags. With no account API there are none. With one but no issuer or
// audience, the routes are registered and each answers 503 "account access
// is not configured": never served without a verified token.
func accountConfigFrom(api, issuer, audience string) (accountConfig, error) {
	if api == "" {
		return accountConfig{}, nil
	}
	known, err := accountHolders(api)
	if err != nil {
		return accountConfig{}, err
	}
	acct := accountConfig{api: api}
	if issuer == "" || audience == "" {
		log.Printf("account routes for %s: --obo-issuer or --obo-audience unset; every account request will answer 503", api)
		return acct, nil
	}
	if u, err := url.Parse(issuer); err != nil || u.Scheme != "https" || u.Host == "" {
		return accountConfig{}, errors.New("--obo-issuer must be an https URL")
	}
	acct.verifier = newOBOVerifier(issuer, audience, known, &http.Client{Timeout: 10 * time.Second}, time.Now)
	log.Printf("account routes for %s: tokens from %s for %s", api, issuer, audience)
	return acct, nil
}
