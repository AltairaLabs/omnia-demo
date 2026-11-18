package main

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/AltairaLabs/omnia-dialhome/server"
	"github.com/go-logr/logr"
)

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
	h, err := newHandler(options{pack: testPack, prompt: defaultPrompt}, s, scriptProvider, logr.Discard())
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

// The transformer order is the one the ToolPolicy in Omnia is written to
// refuse. Today nothing on the dial-home path asks Omnia, so the site raises
// it. This test pins that, so the day the policy is consulted the change is
// seen here rather than discovered in a demo.
func TestAnOrderAboveTheLimitIsRaisedUntilPolicyIsConsulted(t *testing.T) {
	s := newSite()
	got := converse(t, s, "big-order",
		"Raise an urgent work order to regenerate the oil and replace two cooling fans on the transformer, estimated £48,000")
	if !strings.Contains(got, "Raised WO-WE-0412 on WE-GT1") || !strings.Contains(got, "£48000") {
		t.Fatalf("reply = %q", got)
	}
}

// A tool error reaches the engineer as a refusal, not as a made-up result.
func TestAToolErrorIsReportedNotInvented(t *testing.T) {
	s := newSite()
	got := converse(t, s, "bad-cost", "Raise a work order to fix turbine 3, estimated 0 pounds")
	if !strings.Contains(got, "What is the estimated cost") {
		t.Fatalf("reply = %q", got)
	}
	if len(s.raised()) != 0 {
		t.Fatal("an order was raised without a cost")
	}
}
