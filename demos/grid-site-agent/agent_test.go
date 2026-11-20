package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"

	dialhome "github.com/AltairaLabs/omnia-dialhome"
	"github.com/AltairaLabs/omnia-dialhome/server"
	"github.com/go-logr/logr"
)

// siteLimitGBP is the chart's gridSite.workOrderLimitGBP default
// (charts/omnia-demo/values.yaml).
const siteLimitGBP = 25000

// limitMessage is grid-site-policy's @message, as the chart renders it.
var limitMessage = fmt.Sprintf("This work order is above the £%d the site assistant may commit without a human, "+
	"so it was not raised. Ask the control engineer to raise it, and do not split it into smaller orders to "+
	"bring each under the limit. (Wrenmoor Energy delegated authority WE-MNT-03.)", siteLimitGBP)

// sitePolicy stands in for Omnia deciding with the grid-site-policy
// ToolPolicy: it permits the site's tools and refuses a work order above the
// limit. It keeps every question it was asked.
type sitePolicy struct {
	mu    sync.Mutex
	asked []dialhome.PolicyRequest
}

func (p *sitePolicy) Decide(_ context.Context, r dialhome.PolicyRequest) (dialhome.PolicyDecision, error) {
	p.mu.Lock()
	p.asked = append(p.asked, r)
	p.mu.Unlock()
	return decide(r), nil
}

func (p *sitePolicy) questions() []dialhome.PolicyRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]dialhome.PolicyRequest(nil), p.asked...)
}

// decide is grid-site-policy's verdict on one call.
func decide(r dialhome.PolicyRequest) dialhome.PolicyDecision {
	if r.Registry != defaultRegistry {
		return dialhome.PolicyDecision{DeniedBy: "no-matching-permit", Reason: "no policy permits this tool"}
	}
	var args struct {
		Cost *float64 `json:"estimated_cost_gbp"`
	}
	if err := json.Unmarshal(r.Arguments, &args); err != nil {
		// onFailure: deny: a condition that cannot be evaluated refuses.
		return dialhome.PolicyDecision{DeniedBy: "evaluation-failed", Reason: "the call could not be evaluated"}
	}
	if args.Cost != nil && *args.Cost > siteLimitGBP {
		return dialhome.PolicyDecision{DeniedBy: "deny-work-order-above-site-limit", Policy: "grid-site-policy",
			Reason: limitMessage}
	}
	return dialhome.PolicyDecision{Allow: true, Policy: "grid-site-policy"}
}

// The compiled pack the chart ships and the image carries.
const testPack = "../../charts/omnia-demo/files/arena/grid-site-agent-pack.json"

// emitter records one turn's output, as the relay's facade would see it.
type emitter struct {
	mu     sync.Mutex
	chunks strings.Builder
	done   *server.Done
}

func (e *emitter) Chunk(s string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.chunks.WriteString(s)
	return nil
}

func (e *emitter) ToolCall(server.ClientToolCall) (server.ClientToolResult, error) {
	return server.ClientToolResult{IsRejected: true, RejectionReason: "no client tools on site"}, nil
}

func (e *emitter) Done(d server.Done) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.done = &d
	return nil
}

// converse runs one turn on the site agent with the scripted model and
// returns the final reply.
func converse(t *testing.T, s *site, session, msg string) string {
	t.Helper()
	return converseUnder(t, s, &sitePolicy{}, session, msg)
}

// converseUnder is converse with the site's tools put to policy.
func converseUnder(t *testing.T, s *site, policy *sitePolicy, session, msg string) string {
	t.Helper()
	h, err := newHandler(options{pack: testPack, prompt: defaultPrompt, registry: defaultRegistry}, s, scriptProvider,
		governors{policy: policy}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	e := &emitter{}
	if err := h.Converse(context.Background(), server.Turn{SessionID: session, Content: msg}, e); err != nil {
		t.Fatalf("converse %q: %v", msg, err)
	}
	if e.done == nil {
		t.Fatalf("converse %q: no Done", msg)
	}
	if u := e.done.Usage; u != nil && u.CostUSD != 0 {
		t.Fatalf("converse %q: the scripted model reported a cost of $%v", msg, u.CostUSD)
	}
	return e.done.Final
}

// The on-site tools run in this process: each turn below calls one through
// the real pack and the real PromptKit conversation, and the reply carries
// what the site's tool returned.
func TestTurnsRunTheSiteToolsLocally(t *testing.T) {
	s := newSite()
	cases := []struct{ msg, want string }{
		{"How is the site? Any alarms?", "2 active alarms"},
		{"Read turbine 7", "WE-T07 (Turbine 7) is stopped, alarm: Pitch system fault"},
		{"SCADA for WE-GT1 please", "top oil temperature 71 °C"},
		{"Raise an urgent work order to replace the blade B pitch motor on WE-T07, estimated £6,400",
			"Raised WO-WE-0412 on WE-T07 (Turbine 7), urgent, estimated £6400: Replace the blade B pitch motor on WE-T07."},
		{"Open breaker WE-CB3", "I can't operate plant"},
		{"Raise a work order on WE-ZZ9, £100", "That did not go through: no asset \"WE-ZZ9\""},
	}
	for i, c := range cases {
		got := converse(t, s, "session-"+string(rune('a'+i)), c.msg)
		if !strings.Contains(got, c.want) {
			t.Errorf("%q:\n got %q\nwant it to contain %q", c.msg, got, c.want)
		}
	}
	if got := s.raised(); len(got) != 1 || got[0].EstimatedCostGBP != 6400 || got[0].AssetID != "WE-T07" {
		t.Fatalf("work orders raised = %+v, want one £6400 order on WE-T07", got)
	}
}

// The transformer order is above the site's delegated limit. The site asks
// Omnia before the tool runs, Omnia's grid-site-policy refuses it, and the
// work order is never raised: the engineer is told why, in the policy's own
// words.
func TestOmniaRefusesAnOrderAboveTheSiteLimit(t *testing.T) {
	s := newSite()
	policy := &sitePolicy{}
	got := converseUnder(t, s, policy, "big-order",
		"Raise an urgent work order to regenerate the oil and replace two cooling fans on the transformer, estimated £48,000")
	if !strings.HasPrefix(got, "Omnia refused that. This work order is above the £25000") {
		t.Fatalf("reply = %q", got)
	}
	if len(s.raised()) != 0 {
		t.Fatalf("the order was raised: %+v", s.raised())
	}
	q := policy.questions()
	if len(q) != 1 || q[0].Tool != "raise_work_order" || q[0].Registry != defaultRegistry ||
		!strings.Contains(string(q[0].Arguments), `"estimated_cost_gbp":48000`) {
		t.Fatalf("Omnia was asked %+v", q)
	}
}

// With no decision from Omnia, a site tool does not run: not even a read.
func TestNoDecisionNoTool(t *testing.T) {
	s := newSite()
	h, err := newHandler(options{pack: testPack, prompt: defaultPrompt, registry: defaultRegistry}, s, scriptProvider,
		governors{policy: unreachable{}}, logr.Discard())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = h.Close() })
	e := &emitter{}
	if err := h.Converse(context.Background(), server.Turn{SessionID: "down",
		Content: "Raise a work order to replace the pitch motor on WE-T07, £6,400"}, e); err != nil {
		t.Fatal(err)
	}
	if len(s.raised()) != 0 || e.done == nil || !strings.HasPrefix(e.done.Final, "I couldn't do that: the tool did not run") {
		t.Fatalf("raised %+v, reply %+v", s.raised(), e.done)
	}
}

type unreachable struct{}

func (unreachable) Decide(context.Context, dialhome.PolicyRequest) (dialhome.PolicyDecision, error) {
	return dialhome.PolicyDecision{}, fmt.Errorf("%w: %w", dialhome.ErrNoDecision, dialhome.ErrNotConnected)
}

// An order with no usable cost is asked about, not raised with a guess.
func TestAnOrderWithoutACostIsAskedForNotRaised(t *testing.T) {
	s := newSite()
	got := converse(t, s, "bad-cost", "Raise a work order to fix turbine 3, estimated 0 pounds")
	if !strings.Contains(got, "What is the estimated cost") {
		t.Fatalf("reply = %q", got)
	}
	if len(s.raised()) != 0 {
		t.Fatal("an order was raised without a cost")
	}
}
