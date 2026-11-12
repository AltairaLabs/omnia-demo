// Copyright 2026 Altaira Labs.
//
// SPDX-License-Identifier: Apache-2.0

// card-callers holds scripted conversations with Larkmere Mutual's member
// assistant, one after another, so a canary rollout of that agent has traffic
// to judge. Each caller is a synthetic member the card stub knows by email
// (demos/mortgage-underwriting/identitydata.go). The members' lines are fixed
// and do not depend on what the agent says, so the only thing that differs
// between the stable and the candidate version is the agent.
//
// It connects the way the member portal does (demos/demo-sites, assistantProxy):
// in-cluster, to the agent's Service, asserting the signed-in member in the
// edge-trust headers and forwarding a token for them. The members are already
// signed in, as at the portal, so nobody proves who they are in the chat. With
// --issuer-url the callers are an identity provider of their own
// (demos/demoidp), trusted by the install like the portal; without it the
// conversation has no verified user, no memory, and the card tools refuse.
// Each conversation is its own WebSocket, so the mesh routes it to one version
// and it stays there.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/net/websocket"

	"github.com/AltairaLabs/omnia-demo/demos/demoidp"
)

type caller struct {
	id, email, name string
}

// The members, as the card stub knows them (by email).
var callers = []caller{
	{"5120-034", "tomasz.brennan@example.com", "Tomasz Brennan"},
	{"5127-118", "priya.hallworth@example.com", "Priya Hallworth"},
	{"5133-207", "gwen.ashby@example.com", "Gwen Ashby"},
	{"5141-552", "callum.reyes@example.com", "Callum Reyes"},
}

// script is a conversation a signed-in member has with the assistant. One in
// three disputes a payment, which is where an assistant that promises refunds
// (the canary's 2.1) shows itself.
type script struct {
	name  string
	lines func(c caller) []string
}

var scripts = []script{
	{"lost card", func(caller) []string {
		return []string{
			"Hi, I think I've lost my debit card. Can you freeze it please?",
			"How long will it stay frozen, and can I get a new one?",
			"That's everything, thank you.",
		}
	}},
	{"disputed payment", func(caller) []string {
		return []string{
			"There's a payment of £240 on my card from a website I've never used.",
			"Will I get the money back?",
			"Okay, thanks for your help.",
		}
	}},
	{"travel", func(caller) []string {
		return []string{
			"I'm off to Lisbon next week. Is my card active?",
			"Great. And if it's stolen while I'm away, what do I do?",
			"That's all I needed.",
		}
	}},
}

type frame struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func main() {
	agentURL := flag.String("agent-url", "http://pii-chat:8080", "the agent's in-cluster URL")
	agent := flag.String("agent", "pii-chat", "AgentRuntime name")
	namespace := flag.String("namespace", os.Getenv("POD_NAMESPACE"), "the agent's namespace")
	pause := flag.Duration("pause", 20*time.Second, "wait between conversations")
	turnTimeout := flag.Duration("turn-timeout", 90*time.Second, "longest wait for one reply")
	count := flag.Int("count", 0, "conversations to hold, then exit (0: until stopped)")
	issuerURL := flag.String("issuer-url", "", "sign a token for each caller as this issuer; empty: headers only")
	addr := flag.String("addr", ":8080", "where the issuer's JWKS is served, with --issuer-url")
	flag.Parse()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	var iss *demoidp.Issuer
	if *issuerURL != "" {
		var err error
		if iss, err = demoidp.New(*issuerURL); err != nil {
			log.Fatalf("issuer: %v", err)
		}
		mux := http.NewServeMux()
		mux.HandleFunc(demoidp.JWKSPath, iss.ServeJWKS)
		srv := &http.Server{Addr: *addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
		go func() {
			if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				log.Fatalf("jwks: %v", err)
			}
		}()
	}

	for n := 0; *count == 0 || n < *count; n++ {
		c, s := callers[n%len(callers)], scripts[(n/len(callers))%len(scripts)]
		start := time.Now()
		turns, err := converse(ctx, *agentURL, *agent, *namespace, c, s, *turnTimeout, iss)
		if err != nil {
			log.Printf("conversation %d (%s, %s): after %d turns: %v", n+1, c.name, s.name, turns, err)
		} else {
			log.Printf("conversation %d (%s, %s): %d turns in %s", n+1, c.name, s.name, turns, time.Since(start).Round(time.Second))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(*pause):
		}
	}
}

// converse holds one conversation and reports how many turns completed.
func converse(ctx context.Context, agentURL, agent, namespace string, c caller, s script,
	turnTimeout time.Duration, iss *demoidp.Issuer) (int, error) {
	u, err := url.Parse(agentURL)
	if err != nil {
		return 0, err
	}
	u.Scheme = strings.Replace(u.Scheme, "http", "ws", 1)
	u.Path = "/ws"
	u.RawQuery = url.Values{"agent": {agent}, "namespace": {namespace}}.Encode()

	// x/net/websocket always sends an Origin. The facade same-origin-checks it
	// against its own Host, so it is the agent's own origin.
	cfg, err := websocket.NewConfig(u.String(), agentURL)
	if err != nil {
		return 0, err
	}
	cfg.Header.Set("x-user-id", c.id)
	cfg.Header.Set("x-user-email", c.email)
	cfg.Header.Set("x-user-roles", "member")
	if iss != nil {
		tok, err := iss.Sign(c.id, c.email, time.Hour)
		if err != nil {
			return 0, err
		}
		cfg.Header.Set("Authorization", "Bearer "+tok)
	}
	ws, err := cfg.DialContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("connect: %w", err)
	}
	defer func() { _ = ws.Close() }()

	for i, line := range s.lines(c) {
		if err := websocket.JSON.Send(ws, map[string]string{"type": "message", "content": line}); err != nil {
			return i, fmt.Errorf("send: %w", err)
		}
		if err := awaitDone(ws, turnTimeout); err != nil {
			return i, err
		}
	}
	return len(s.lines(c)), nil
}

// awaitDone reads frames until the reply is complete.
func awaitDone(ws *websocket.Conn, timeout time.Duration) error {
	if err := ws.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	for {
		var raw []byte
		if err := websocket.Message.Receive(ws, &raw); err != nil {
			return fmt.Errorf("receive: %w", err)
		}
		var f frame
		if json.Unmarshal(raw, &f) != nil {
			continue
		}
		switch f.Type {
		case "done":
			return nil
		case "error":
			if f.Error != nil {
				return fmt.Errorf("agent error %s: %s", f.Error.Code, f.Error.Message)
			}
			return errors.New("agent error")
		}
	}
}
