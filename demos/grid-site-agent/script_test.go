package main

import (
	"context"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/AltairaLabs/PromptKit/runtime/v2/providers"
	"github.com/AltairaLabs/PromptKit/runtime/v2/types"
)

// Every streamed chunk is valid UTF-8, wherever a "£" or "°" falls: gRPC
// refuses to carry a chunk that is not. The tool result is padded so a
// multi-byte rune straddles each 20-byte boundary in turn.
func TestStreamedChunksAreValidUTF8(t *testing.T) {
	for pad := 0; pad < chunkBytes; pad++ {
		summary := strings.Repeat("x", pad) + "£48,000 at 71 °C, £6,400 at 84 °C"
		p := newScriptedProvider().(*scriptedProvider)
		res := &types.MessageToolResult{Name: "read_scada"}
		res.Parts = []types.ContentPart{types.NewTextPart(`{"summary":"` + summary + `"}`)}
		req := providers.PredictionRequest{Messages: []types.Message{
			{Role: "user", Content: "Read WE-GT1"},
			{Role: "tool", ToolResult: res},
		}}
		ch, err := p.PredictStreamWithTools(context.Background(), req, nil, "auto")
		if err != nil {
			t.Fatal(err)
		}
		var got strings.Builder
		for c := range ch {
			if !utf8.ValidString(c.Delta) {
				t.Fatalf("pad %d: chunk %q is not valid UTF-8", pad, c.Delta)
			}
			if c.CostInfo != nil && c.CostInfo.TotalCost != 0 {
				t.Fatalf("pad %d: the script reported a cost", pad)
			}
			got.WriteString(c.Delta)
		}
		if got.String() != summary {
			t.Fatalf("pad %d: streamed %q, want %q", pad, got.String(), summary)
		}
	}
}

func TestWorkIn(t *testing.T) {
	cases := map[string]string{
		"Raise a work order to replace the pitch motor on turbine 7 for £6,400": "Replace the pitch motor on turbine 7",
		"Raise a work order to fix the breaker at £900":                         "Fix the breaker",
		"Raise a work order, estimated £100":                                    "Investigate and repair WE-CB3",
		"Raise a work order to élever the fan, £10":                             "Élever the fan",
	}
	for in, want := range cases {
		if got := workIn(in, "WE-CB3"); got != want {
			t.Errorf("workIn(%q) = %q, want %q", in, got, want)
		}
	}
}
