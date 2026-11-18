package main

import (
	"errors"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	"github.com/AltairaLabs/PromptKit/sdk/v2"
)

// The site's SCADA and maintenance system, as the on-site tools see them.
//
// Everything here is fictional and deterministic: the SCADA values are one
// fixed snapshot, so the same question gets the same figures on every take,
// and work orders are numbered from a fixed start each time the process
// starts. Nothing here reaches a network; that is the point of the demo.

const (
	siteName = "Wrenmoor Edge"
	// asOf is the snapshot every reading reports. Fixed, not the clock.
	asOf = "2026-11-18T09:30:00Z"
	// firstWorkOrder is the number the first order raised gets.
	firstWorkOrder = 412
)

// point is one measured SCADA value.
type point struct {
	Point string  `json:"point"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// asset is one item of plant.
type asset struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	State  string  `json:"state"`
	Alarm  string  `json:"alarm,omitempty"`
	Points []point `json:"-"`
}

// assets is Wrenmoor Edge as SCADA reports it at asOf: two active alarms,
// one cheap to fix (turbine 7's pitch motor) and one expensive (the grid
// transformer running hot).
var assets = []asset{
	{ID: "WE-GT1", Name: "Grid transformer 33/132 kV", State: "in service",
		Alarm: "Top-oil temperature high (alarm at 70 °C)",
		Points: []point{
			{"load", 21.6, "MVA"}, {"rating", 40, "MVA"},
			{"top_oil_temperature", 71, "°C"}, {"winding_hot_spot_temperature", 84, "°C"},
			{"tap_position", 7, ""}, {"cooling_fans_running", 2, "of 4"},
		}},
	{ID: "WE-CB3", Name: "Feeder 3 circuit breaker, 33 kV", State: "closed",
		Points: []point{
			{"current", 312, "A"}, {"operations_last_12_months", 4, ""},
		}},
	{ID: "WE-T03", Name: "Turbine 3", State: "running",
		Points: []point{
			{"active_power", 2780, "kW"}, {"wind_speed", 11.4, "m/s"},
			{"rotor_speed", 14.1, "rpm"}, {"nacelle_temperature", 31, "°C"},
		}},
	{ID: "WE-T07", Name: "Turbine 7", State: "stopped",
		Alarm: "Pitch system fault, blade B (PS-214)",
		Points: []point{
			{"active_power", 0, "kW"}, {"wind_speed", 11.2, "m/s"},
			{"blade_b_pitch_motor_current", 0, "A"}, {"nacelle_temperature", 18, "°C"},
		}},
	{ID: "WE-MET1", Name: "Met mast", State: "reporting",
		Points: []point{
			{"wind_speed", 11.4, "m/s"}, {"wind_direction", 247, "°"},
			{"air_temperature", 6.1, "°C"},
		}},
}

// priorities a work order may carry.
var priorities = map[string]bool{"routine": true, "urgent": true, "emergency": true}

// site holds the maintenance system's state: the work orders raised since the
// process started.
type site struct {
	mu     sync.Mutex
	next   int
	orders []workOrder
}

type workOrder struct {
	WorkOrder        string `json:"work_order"`
	AssetID          string `json:"asset_id"`
	Priority         string `json:"priority"`
	Description      string `json:"description"`
	EstimatedCostGBP int64  `json:"estimated_cost_gbp"`
	Status           string `json:"status"`
}

func newSite() *site { return &site{next: firstWorkOrder} }

// register gives a conversation the site's tools. Each runs here, in this
// process, inside the site network: none of them crosses the dial-home
// channel.
func (s *site) register(conv *sdk.Conversation) error {
	conv.OnTool("list_assets", s.listAssets)
	conv.OnTool("read_scada", s.readSCADA)
	conv.OnTool("raise_work_order", s.raiseWorkOrder)
	return nil
}

func findAsset(id string) (asset, bool) {
	id = strings.ToUpper(strings.TrimSpace(id))
	for _, a := range assets {
		if a.ID == id {
			return a, true
		}
	}
	return asset{}, false
}

func assetIDs() string {
	ids := make([]string, 0, len(assets))
	for _, a := range assets {
		ids = append(ids, a.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ", ")
}

func (s *site) listAssets(map[string]any) (any, error) {
	var alarms []string
	for _, a := range assets {
		if a.Alarm != "" {
			alarms = append(alarms, fmt.Sprintf("%s (%s): %s", a.ID, a.Name, a.Alarm))
		}
	}
	summary := fmt.Sprintf("%s has %d assets and %d active alarms: %s.",
		siteName, len(assets), len(alarms), strings.Join(alarms, "; "))
	return map[string]any{"site": siteName, "as_of": asOf, "assets": assets, "summary": summary}, nil
}

func (s *site) readSCADA(args map[string]any) (any, error) {
	id, _ := args["asset_id"].(string)
	a, ok := findAsset(id)
	if !ok {
		return nil, fmt.Errorf("no asset %q at %s; the assets are %s", id, siteName, assetIDs())
	}
	parts := make([]string, 0, len(a.Points))
	for _, p := range a.Points {
		parts = append(parts, strings.TrimSpace(fmt.Sprintf("%s %g %s", strings.ReplaceAll(p.Point, "_", " "), p.Value, p.Unit)))
	}
	summary := fmt.Sprintf("%s (%s) is %s", a.ID, a.Name, a.State)
	if a.Alarm != "" {
		summary += ", alarm: " + a.Alarm
	}
	summary += ". " + strings.Join(parts, ", ") + "."
	return map[string]any{
		"asset_id": a.ID, "name": a.Name, "state": a.State, "alarm": a.Alarm,
		"as_of": asOf, "points": a.Points, "summary": summary,
	}, nil
}

func (s *site) raiseWorkOrder(args map[string]any) (any, error) {
	id, _ := args["asset_id"].(string)
	a, ok := findAsset(id)
	if !ok {
		return nil, fmt.Errorf("no asset %q at %s; the assets are %s", id, siteName, assetIDs())
	}
	priority, _ := args["priority"].(string)
	if !priorities[priority] {
		return nil, fmt.Errorf("priority must be routine, urgent or emergency, not %q", priority)
	}
	desc, _ := args["summary"].(string)
	if strings.TrimSpace(desc) == "" {
		return nil, errors.New("a work order needs a summary of the work")
	}
	cost, err := wholePounds(args["estimated_cost_gbp"])
	if err != nil {
		return nil, err
	}

	s.mu.Lock()
	wo := workOrder{
		WorkOrder: fmt.Sprintf("WO-WE-%04d", s.next), AssetID: a.ID, Priority: priority,
		Description: desc, EstimatedCostGBP: cost, Status: "raised",
	}
	s.next++
	s.orders = append(s.orders, wo)
	s.mu.Unlock()

	return map[string]any{
		"work_order": wo.WorkOrder, "asset_id": wo.AssetID, "priority": wo.Priority,
		"estimated_cost_gbp": wo.EstimatedCostGBP, "status": wo.Status,
		"summary": fmt.Sprintf("Raised %s on %s (%s), %s, estimated £%d: %s.",
			wo.WorkOrder, a.ID, a.Name, wo.Priority, wo.EstimatedCostGBP, wo.Description),
	}, nil
}

// wholePounds reads a positive whole number of pounds. Tool arguments arrive
// as JSON, so a number is a float64; a fractional one is refused rather than
// rounded, since money that silently changes is worse than a refusal.
func wholePounds(v any) (int64, error) {
	f, ok := v.(float64)
	if !ok {
		return 0, errors.New("estimated_cost_gbp must be a whole number of pounds")
	}
	if f <= 0 || f != math.Trunc(f) || f > math.MaxInt32 {
		return 0, fmt.Errorf("estimated_cost_gbp must be a positive whole number of pounds, not %v", f)
	}
	return int64(f), nil
}

// raised returns a copy of the work orders raised so far.
func (s *site) raised() []workOrder {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]workOrder(nil), s.orders...)
}
