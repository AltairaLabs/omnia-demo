package main

import (
	"fmt"
	"strings"
	"testing"
)

func TestReadSCADA(t *testing.T) {
	s := newSite()
	out, err := s.readSCADA(map[string]any{"asset_id": "we-gt1"})
	if err != nil {
		t.Fatal(err)
	}
	m := out.(map[string]any)
	if m["asset_id"] != "WE-GT1" || m["as_of"] != asOf || !strings.Contains(m["alarm"].(string), "Top-oil") {
		t.Fatalf("read = %+v", m)
	}
	if _, err := s.readSCADA(map[string]any{"asset_id": "WE-X1"}); err == nil || !strings.Contains(err.Error(), "WE-T07") {
		t.Fatalf("unknown asset error = %v, want it to list the assets", err)
	}
}

func TestListAssetsHidesPointsAndCountsAlarms(t *testing.T) {
	out, _ := newSite().listAssets(nil)
	m := out.(map[string]any)
	if !strings.Contains(m["summary"].(string), "5 assets and 2 active alarms") {
		t.Fatalf("summary = %q", m["summary"])
	}
}

func TestRaiseWorkOrderValidatesAndNumbers(t *testing.T) {
	s := newSite()
	ok := map[string]any{"asset_id": "WE-T07", "priority": "urgent", "summary": "Replace pitch motor", "estimated_cost_gbp": 6400.0}
	bad := []map[string]any{
		{"asset_id": "WE-NOPE", "priority": "urgent", "summary": "x", "estimated_cost_gbp": 1.0},
		{"asset_id": "WE-T07", "priority": "whenever", "summary": "x", "estimated_cost_gbp": 1.0},
		{"asset_id": "WE-T07", "priority": "urgent", "summary": " ", "estimated_cost_gbp": 1.0},
		{"asset_id": "WE-T07", "priority": "urgent", "summary": "x", "estimated_cost_gbp": 10.5},
		{"asset_id": "WE-T07", "priority": "urgent", "summary": "x", "estimated_cost_gbp": -3.0},
		{"asset_id": "WE-T07", "priority": "urgent", "summary": "x", "estimated_cost_gbp": "6400"},
	}
	for _, args := range bad {
		if _, err := s.raiseWorkOrder(args); err == nil {
			t.Errorf("%v: raised, want refused", args)
		}
	}
	for want := firstWorkOrder; want < firstWorkOrder+2; want++ {
		out, err := s.raiseWorkOrder(ok)
		if err != nil {
			t.Fatal(err)
		}
		if got := out.(map[string]any)["work_order"]; got != fmt.Sprintf("WO-WE-%04d", want) {
			t.Fatalf("work order = %v, want number %d", got, want)
		}
	}
	if n := len(s.raised()); n != 2 {
		t.Fatalf("raised %d, want 2", n)
	}
}
