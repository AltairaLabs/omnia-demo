// Command personal-agent is the protocol side of "Ellen's assistant", a
// Personal Agent under the Personal Agent Protocol (PAP 1.0,
// https://personalagentprotocol.org/docs/1.0/spec). It serves the agent's
// public identity documents, holds its keys and its User IDs, discovers
// Companies, holds their conversations with a background Reader each, and
// answers the HTTP tools Ellen's assistant (an Omnia agent) calls. The
// assistant's page reads /api/updates for what arrives between turns.
//
// STAND-IN (docs/stand-ins.md, "Personal Agent").
//
// What it fakes: the person's own AI assistant, and the vendor that runs it.
//
// What the real system is: a consumer Personal Agent (Meta's Muse is the
// only one today), run by another company on its own infrastructure, with
// its own users and its own sign-in.
//
// What changes downstream because of the fake:
//   - It runs on the same Omnia install as the Companies it talks to, so the
//     dashboard shows both sides of a conversation. The two sides share
//     nothing but public HTTPS: the Companies' facades fetch agent.json and
//     jwks.json from the agent's public hostname, and the agent reaches the
//     Companies at theirs, exactly as they would reach Muse.
//   - Its identity keys are in a cluster Secret, not an HSM.
//   - It has one user, Ellen Whitcomb.
//   - This tool service, not the model, holds the keys and the User IDs.
//     The model sees conversation IDs, Company names and messages; no tool
//     answer carries a User ID, a key or a Session JWT.
//   - The sign-in redirect lands on a public callback that cannot tell who
//     completed it (PAP §4.2, §10.8). A real Personal Agent would receive
//     it in its own app, signed in as the User who began the sign-in, or
//     on the device that opened the link. Here a forwarded link completes
//     the sign-in for Ellen, so the Company's consent page (prompt=consent)
//     is the only guard; nothing here works around it.
//   - Sign-in tokens are sealed in Redis under a key from a cluster Secret
//     (--token-key-file), not an HSM or a vendor's key service.
//
// Written from the spec. Nothing is copied from Omnia's own test Personal
// Agent, which is under a commercial licence (this repository is public).
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/conversation"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/identity"
	"github.com/AltairaLabs/omnia-demo/demos/personal-agent/signin"
)

func main() {
	identityDir := flag.String("identity-dir", "/var/run/personal-agent/identity", "directory holding es256.pem and ps384.pem (PKCS#8)")
	baseURL := flag.String("base-url", "https://assistant.example.com", "the https origin agent.json and jwks.json are served from")
	redisURL := flag.String("redis-url", "", "Redis for User IDs, conversations and events (redis:// or rediss://); empty keeps them in memory, lost on restart")
	addr := flag.String("addr", ":8080", "listen address")
	testIdentityDir := flag.String("test-identity-dir", "", "if set, serve this directory's files at /poppy-test/ (the PAP validator's test identity)")
	known := flag.String("known-companies", "", "comma-separated domains Ellen has accounts at")
	tokenKeyFile := flag.String("token-key-file", "", "file holding the key sign-in tokens are sealed under in Redis: one line of standard base64, 32 bytes; required with --redis-url")
	flag.Parse()

	id, err := identity.Load(*identityDir, *baseURL)
	if err != nil {
		log.Fatal(err)
	}
	var st conversation.Store
	if *redisURL != "" {
		rs, err := conversation.NewRedisStore(*redisURL, id, time.Now)
		if err != nil {
			log.Fatal(err)
		}
		st = rs
	} else {
		log.Print("no --redis-url: User IDs and conversations are kept in memory and lost on restart")
		st = conversation.NewMemoryStore(id, time.Now)
	}
	tokens, err := tokenStore(*redisURL, *tokenKeyFile)
	if err != nil {
		log.Fatal(err)
	}

	var companies []string
	for _, d := range strings.Split(*known, ",") {
		if d = strings.TrimSpace(d); d != "" {
			companies = append(companies, d)
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	s := newServer(ctx, config{
		Identity: id, Store: st, HTTP: companyClient(),
		KnownCompanies: companies, TestIdentityDir: *testIdentityDir,
		SignIn: signin.NewFlow(issuerClient(), id, tokens),
	})
	s.resume()

	srv := &http.Server{Addr: *addr, Handler: s.handler(), ReadHeaderTimeout: 10 * time.Second}
	// ListenAndServe returns as soon as Shutdown starts, while requests are
	// still finishing; shutdownDone closes once they have (or 10s passed).
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("personal-agent listening on %s as %s; known companies %v", *addr, id.ClientID, companies)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	<-shutdownDone
	s.wait()
}

// tokenStore is where sign-in tokens are kept: sealed in Redis under the
// key in keyFile, which is then required, or in memory without Redis, lost
// on restart.
func tokenStore(redisURL, keyFile string) (signin.TokenStore, error) {
	if redisURL == "" {
		log.Print("no --redis-url: sign-in tokens are kept in memory and lost on restart")
		return signin.NewMemoryTokenStore(), nil
	}
	if keyFile == "" {
		return nil, errors.New("--token-key-file is required with --redis-url: sign-in tokens are sealed under it")
	}
	key, err := signin.LoadKey(keyFile)
	if err != nil {
		return nil, err
	}
	return signin.NewRedisTokenStore(redisURL, key)
}

func transport() *http.Transport {
	return &http.Transport{
		Proxy:                 http.ProxyFromEnvironment,
		DialContext:           (&net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
		ForceAttemptHTTP2:     true,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 60 * time.Second,
		IdleConnTimeout:       90 * time.Second,
		MaxIdleConns:          20,
	}
}

// companyClient reaches the Companies. It has no overall timeout, because
// event streams stay open for minutes; ResponseHeaderTimeout covers a read
// held up to its 30s wait (§8.5).
func companyClient() *http.Client {
	return &http.Client{Transport: transport()}
}

// issuerClient reaches the Companies' issuers: metadata, token exchange,
// refresh and revocation. Each is one short request, so a hung issuer is
// cut off after 30s (a refresh, which holds up that User's requests, after
// the Flow's own 15s).
func issuerClient() *http.Client {
	return &http.Client{Transport: transport(), Timeout: 30 * time.Second}
}
