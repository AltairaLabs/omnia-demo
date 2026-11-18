package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/providers/mock"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// The scripted model: PromptKit's mock provider, choosing its next move from
// the engineer's last message instead of from a model. It calls the site's
// real tools with the arguments the message names, then answers with what the
// tool returned, so a take is repeatable and costs nothing. The tools, the
// dial-home channel and Omnia's session record are live; only the model's
// choice of what to call is scripted. Say so if asked.
//
// Lines it understands (any wording that contains the cue works):
//
//	"How is the site?" / "any alarms?"          list_assets
//	"Read turbine 7" / "SCADA for WE-GT1"       read_scada
//	"Raise a work order to <work> on <asset>,   raise_work_order
//	 estimated £6,400" (add "urgent" or
//	 "emergency" to set the priority)
//	"Open breaker WE-CB3"                       a refusal: it never operates plant

// scriptedProvider is a mock provider that sees each request's messages
// before the mock asks the script for its turn.
type scriptedProvider struct {
	*mock.ToolProvider
	script *script
}

func newScriptedProvider() providers.Provider {
	s := &script{}
	return &scriptedProvider{
		ToolProvider: mock.NewToolProviderWithRepository("site-script", "grid-ops-script", false, s),
		script:       s,
	}
}

func (p *scriptedProvider) Predict(ctx context.Context, req providers.PredictionRequest) (providers.PredictionResponse, error) {
	p.script.observe(req.Messages)
	return p.ToolProvider.Predict(ctx, req)
}

func (p *scriptedProvider) PredictStream(ctx context.Context, req providers.PredictionRequest) (<-chan providers.StreamChunk, error) {
	p.script.observe(req.Messages)
	return p.ToolProvider.PredictStream(ctx, req)
}

func (p *scriptedProvider) PredictWithTools(ctx context.Context, req providers.PredictionRequest,
	tools providers.ProviderTools, toolChoice string,
) (providers.PredictionResponse, []types.MessageToolCall, error) {
	p.script.observe(req.Messages)
	resp, calls, err := p.ToolProvider.PredictWithTools(ctx, req, tools, toolChoice)
	// The mock prices its made-up tokens; a script spends nothing, and a
	// non-zero cost would be recorded on the session and count against the
	// workspace's budget.
	resp.CostInfo = nil
	return resp, calls, err
}

// PredictStreamWithTools streams the scripted turn itself rather than through
// the mock's own streaming, which cuts the text every 20 bytes: a cut through
// a "£" or a "°" sends invalid UTF-8, and gRPC refuses to carry it to Omnia
// (https://github.com/AltairaLabs/PromptKit/issues/2799). Here every chunk
// ends on a rune boundary.
func (p *scriptedProvider) PredictStreamWithTools(ctx context.Context, req providers.PredictionRequest,
	tools any, toolChoice string,
) (<-chan providers.StreamChunk, error) {
	p.script.observe(req.Messages)
	resp, calls, err := p.PredictWithTools(ctx, req, tools, toolChoice)
	if err != nil {
		return nil, err
	}
	out := make(chan providers.StreamChunk, providers.DefaultStreamBufferSize)
	go func() {
		defer close(out)
		content := resp.Content
		for start := 0; start < len(content); {
			end := min(start+chunkBytes, len(content))
			for end < len(content) && !utf8.RuneStart(content[end]) {
				end++
			}
			out <- providers.StreamChunk{Delta: content[start:end], Content: content[:end]}
			start = end
		}
		finish := types.FinishReasonStop
		if len(calls) > 0 {
			finish = types.FinishReasonToolUse
		}
		out <- providers.StreamChunk{Content: content, ToolCalls: calls, FinishReason: &finish}
	}()
	return out, nil
}

// chunkBytes is roughly how much of the reply each streamed chunk carries.
const chunkBytes = 20

// script is the mock's response repository. Each conversation has its own
// provider and so its own script, and a conversation runs one turn at a time.
type script struct {
	mu     sync.Mutex
	user   string         // the engineer's last message
	result *types.Message // the last tool result since it, if any
}

// observe records the last user message and the last tool result after it.
func (s *script) observe(msgs []types.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.user, s.result = "", nil
	for i := range msgs {
		switch msgs[i].Role {
		case "user":
			s.user, s.result = msgs[i].GetContent(), nil
		case "tool":
			s.result = &msgs[i]
		}
	}
}

func (s *script) GetResponse(ctx context.Context, p mock.ResponseParams) (string, error) {
	t, err := s.GetTurn(ctx, p)
	if err != nil {
		return "", err
	}
	return t.Content, nil
}

func (s *script) GetTurn(context.Context, mock.ResponseParams) (*mock.Turn, error) {
	s.mu.Lock()
	user, result := s.user, s.result
	s.mu.Unlock()
	if result != nil {
		return text(answer(result)), nil
	}
	return plan(user), nil
}

func text(s string) *mock.Turn { return &mock.Turn{Type: "text", Content: s} }

func call(name string, args map[string]any) *mock.Turn {
	return &mock.Turn{Type: "tool_calls", ToolCalls: []mock.ToolCall{{Name: name, Arguments: args}}}
}

// answer turns a tool's result into the reply: the summary the site's tools
// write, or why the call did not happen.
func answer(m *types.Message) string {
	if m.ToolResult != nil && m.ToolResult.Error != "" {
		return "That did not go through: " + m.ToolResult.Error
	}
	content := m.GetContent()
	var out struct {
		Summary string `json:"summary"`
	}
	if err := json.Unmarshal([]byte(content), &out); err == nil && out.Summary != "" {
		return out.Summary
	}
	return "That did not go through: " + content
}

const help = "I can give you the site's alarms, read any asset's SCADA values " +
	"(e.g. WE-T07 or WE-GT1), and raise work orders. What do you need?"

var (
	assetPattern   = regexp.MustCompile(`(?i)\bWE-[A-Z]+\d+\b`)
	turbinePattern = regexp.MustCompile(`(?i)\bturbine\s+(\d{1,2})\b`)
	costPattern    = regexp.MustCompile(`(?i)£\s*([\d,]+)|([\d,]+)\s*(?:gbp|pounds)\b`)
	assetWords     = []struct{ word, id string }{{"transformer", "WE-GT1"}, {"breaker", "WE-CB3"}, {"met mast", "WE-MET1"}}
	operateCues    = []string{"open ", "close ", "trip ", "start ", "stop ", "switch ", "change the tap", "reset "}
	readCues       = []string{"read", "scada", "reading", "values", "status of", "how is we-", "how's we-", "check "}
	overviewCues   = []string{"site", "alarm", "overview", "anything wrong", "what's wrong", "faults"}
)

// plan picks the turn's move from the engineer's message.
func plan(user string) *mock.Turn {
	msg := strings.ToLower(user)
	id := assetIn(user)
	switch {
	case strings.Contains(msg, "work order"):
		return planWorkOrder(user, id)
	case containsAny(msg, operateCues) && id != "":
		return text("I can't operate plant: switching on " + id +
			" is done by an authorised person under the site's safety rules. I can read its SCADA values or raise a work order.")
	case id != "" && (containsAny(msg, readCues) || len(strings.Fields(msg)) <= 3):
		return call("read_scada", map[string]any{"asset_id": id})
	case containsAny(msg, overviewCues):
		return call("list_assets", map[string]any{})
	default:
		return text(help)
	}
}

func planWorkOrder(user, id string) *mock.Turn {
	if id == "" {
		return text("Which asset is the work on? The assets are " + assetIDs() + ".")
	}
	cost, ok := costIn(user)
	if !ok {
		return text("What is the estimated cost of the work on " + id + ", in pounds?")
	}
	return call("raise_work_order", map[string]any{
		"asset_id":           id,
		"priority":           priorityIn(user),
		"summary":            workIn(user, id),
		"estimated_cost_gbp": cost,
	})
}

// assetIn finds the asset a message names: its id, "turbine N", or a word for
// one of the substation's assets.
func assetIn(user string) string {
	if m := assetPattern.FindString(user); m != "" {
		return strings.ToUpper(m)
	}
	if m := turbinePattern.FindStringSubmatch(user); m != nil {
		n, _ := strconv.Atoi(m[1])
		return fmt.Sprintf("WE-T%02d", n)
	}
	msg := strings.ToLower(user)
	for _, w := range assetWords {
		if strings.Contains(msg, w.word) {
			return w.id
		}
	}
	return ""
}

func costIn(user string) (int64, bool) {
	m := costPattern.FindStringSubmatch(user)
	if m == nil {
		return 0, false
	}
	digits := strings.ReplaceAll(m[1]+m[2], ",", "")
	n, err := strconv.ParseInt(digits, 10, 64)
	return n, err == nil && n > 0
}

func priorityIn(user string) string {
	msg := strings.ToLower(user)
	switch {
	case strings.Contains(msg, "emergency"):
		return "emergency"
	case strings.Contains(msg, "urgent"):
		return "urgent"
	default:
		return "routine"
	}
}

// workIn is the work a message describes: what follows "to" after "work
// order", up to the cost, e.g. "Replace the blade B pitch motor on WE-T07".
func workIn(user, id string) string {
	msg := user
	if i := strings.Index(strings.ToLower(msg), "work order"); i >= 0 {
		msg = msg[i+len("work order"):]
	}
	if i := strings.Index(strings.ToLower(msg), " to "); i >= 0 {
		msg = msg[i+len(" to "):]
	}
	for _, stop := range []string{" for £", " at £", ",", " - ", "(", "£", " estimated"} {
		if i := strings.Index(strings.ToLower(msg), strings.ToLower(stop)); i >= 0 {
			msg = msg[:i]
		}
	}
	msg = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(msg), "."))
	if msg == "" {
		return "Investigate and repair " + id
	}
	r, n := utf8.DecodeRuneInString(msg)
	return string(unicode.ToUpper(r)) + msg[n:]
}

func containsAny(s string, cues []string) bool {
	for _, c := range cues {
		if strings.Contains(s, c) {
			return true
		}
	}
	return false
}
