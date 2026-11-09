package main

import (
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"
)

// Wrenmoor Energy tool implementations. See wrenmoordata.go for the fixtures.
//
// Two demos share this state:
//
//	incidentAgent  /ops/*   alerts, logs, deploy history, and the actions on
//	                        them: rollback and scale (reversible), namespace
//	                        deletion and database failover (irreversible)
//	dataAnalyst    /grid/*  feeders, half-hourly load, consumption, and the
//	                        daily summaries the scheduled digest walks
//
// The ops state is MUTABLE and held in memory, so a rollback is visible in the
// deploy history and clears the alert it fixed — the agent can check its own
// work. A pod restart puts everything back to the seeded incident, which is
// what a demo wants between takes.
//
// THE IRREVERSIBLE ENDPOINTS DO WHAT THEY SAY, like claim_settle does. The
// control under demonstration is the ToolPolicy, evaluated before dispatch, so
// a call to one of them never arrives here. Refusing here as well would make
// it impossible to tell whether the policy or the back end said no.

// wrenmoor holds the mutable on-call state. One mutex: the stub serves one
// demo at a time and a finer lock would only add places to get it wrong.
type wrenmoor struct {
	mu  sync.Mutex
	now func() time.Time

	services   map[string]*opsService
	alerts     []alert
	logs       map[string][]logLine
	deploys    map[string][]deploy
	namespaces map[string]string // namespace -> phase
	primaries  map[string]string // database cluster -> primary
}

func newWrenmoor(now func() time.Time) *wrenmoor {
	w := &wrenmoor{
		now:       now,
		services:  seedServices(),
		alerts:    seedAlerts(),
		logs:      seedLogs(),
		deploys:   seedDeploys(),
		primaries: seedDatabases(),
		namespaces: map[string]string{
			"platform-prod":      "Active",
			"customer-prod":      "Active",
			"outage-map-staging": "Terminating",
		},
	}
	return w
}

// register adds the Wrenmoor Energy endpoints to mux.
func (w *wrenmoor) register(mux *http.ServeMux) {
	mux.HandleFunc("/ops/alerts", jsonTool("get_alerts", w.getAlerts))
	mux.HandleFunc("/ops/logs", jsonTool("get_logs", w.getLogs))
	mux.HandleFunc("/ops/deploys", jsonTool("get_deploy_history", w.getDeployHistory))
	mux.HandleFunc("/ops/rollback", jsonTool("rollback_deploy", w.rollbackDeploy))
	mux.HandleFunc("/ops/scale", jsonTool("scale_deployment", w.scaleDeployment))
	mux.HandleFunc("/ops/delete-namespace", jsonTool("delete_namespace", w.deleteNamespace))
	mux.HandleFunc("/ops/failover-database", jsonTool("failover_database", w.failoverDatabase))
	mux.HandleFunc("/grid/feeders", jsonTool("list_feeders", w.listFeeders))
	mux.HandleFunc("/grid/load", jsonTool("get_feeder_load", w.getFeederLoad))
	mux.HandleFunc("/grid/consumption", jsonTool("get_consumption", w.getConsumption))
	mux.HandleFunc("/grid/daily-summaries", jsonTool("get_daily_summaries", w.getDailySummaries))
}

func (w *wrenmoor) stamp() string {
	return w.now().UTC().Format(time.RFC3339)
}

// ── On-call: reads ──────────────────────────────────────────────────────────

func (w *wrenmoor) getAlerts(req toolRequest) (any, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := []alert{}
	for _, a := range w.alerts {
		if req.Service != "" && a.Service != req.Service {
			continue
		}
		if req.Status != "" && a.Status != req.Status {
			continue
		}
		out = append(out, a)
	}
	return map[string]any{"alerts": out}, http.StatusOK
}

func (w *wrenmoor) getLogs(req toolRequest) (any, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.services[req.Service]; !ok {
		return errorBody("unknown service; known services are meter-ingest, grid-telemetry, billing-api, payments, outage-map"), http.StatusNotFound
	}
	lines := w.logs[req.Service]
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if len(lines) > limit {
		lines = lines[len(lines)-limit:]
	}
	return map[string]any{"service": req.Service, "lines": append([]logLine{}, lines...)}, http.StatusOK
}

func (w *wrenmoor) getDeployHistory(req toolRequest) (any, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	svc, ok := w.services[req.Service]
	if !ok {
		return errorBody("unknown service"), http.StatusNotFound
	}
	history := w.deploys[req.Service]
	newestFirst := make([]deploy, 0, len(history))
	for i := len(history) - 1; i >= 0; i-- {
		newestFirst = append(newestFirst, history[i])
	}
	return map[string]any{
		"service":          svc.Name,
		"namespace":        svc.Namespace,
		"replicas":         svc.Replicas,
		"current_revision": currentRevision(history),
		"history":          newestFirst,
	}, http.StatusOK
}

func currentRevision(history []deploy) int {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Status == deployCurrent {
			return history[i].Revision
		}
	}
	return 0
}

// ── On-call: reversible actions ─────────────────────────────────────────────

// rollbackDeploy rolls a service back by rolling FORWARD to a new revision
// carrying an earlier image, as a real rollout does. That is what makes it
// reversible: nothing is lost, and rolling forward again is one more call.
func (w *wrenmoor) rollbackDeploy(req toolRequest) (any, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	svc, ok := w.services[req.Service]
	if !ok {
		return errorBody("unknown service"), http.StatusNotFound
	}
	history := w.deploys[req.Service]
	from := currentRevision(history)
	if len(history) < 2 {
		return errorBody("no earlier revision to roll back to"), http.StatusConflict
	}

	var target *deploy
	for i := len(history) - 1; i >= 0; i-- {
		d := &history[i]
		if req.ToRevision > 0 && d.Revision != req.ToRevision {
			continue
		}
		if req.ToRevision == 0 && (d.Revision == from || d.Status == deployRolledBack) {
			continue
		}
		target = d
		break
	}
	if target == nil {
		return errorBody(fmt.Sprintf("revision %d not found in this service's history", req.ToRevision)), http.StatusNotFound
	}
	if target.Revision == from {
		return errorBody("that revision is already current"), http.StatusConflict
	}

	for i := range history {
		if history[i].Revision == from {
			history[i].Status = deployRolledBack
		}
	}
	next := history[len(history)-1].Revision + 1
	change := fmt.Sprintf("Rollback of revision %d to the image of revision %d", from, target.Revision)
	if req.Reason != "" {
		change += ": " + req.Reason
	}
	history = append(history, deploy{
		Revision: next, Image: target.Image, DeployedAt: w.stamp(),
		DeployedBy: "incident-agent", Change: change, Status: deployCurrent,
	})
	w.deploys[req.Service] = history

	if svc.ErrorAlert != "" {
		for i := range w.alerts {
			if w.alerts[i].ID == svc.ErrorAlert {
				w.alerts[i].Status = alertResolved
			}
		}
	}
	w.logs[req.Service] = append(w.logs[req.Service],
		logLine{At: w.stamp(), Level: "info", Message: fmt.Sprintf("rollout: revision %d (%s) ready %d/%d", next, target.Image, svc.Replicas, svc.Replicas)},
	)

	return map[string]any{
		"service":            svc.Name,
		"rolled_back_from":   from,
		"now_serving":        next,
		"image":              target.Image,
		"status":             "rolled_out",
		"undo":               fmt.Sprintf("rollback_deploy with to_revision %d restores the previous image", from),
		"resolved_alert_ids": resolvedFor(svc),
	}, http.StatusOK
}

func resolvedFor(svc *opsService) []string {
	if svc.ErrorAlert == "" {
		return []string{}
	}
	return []string{svc.ErrorAlert}
}

// maxReplicas caps a scale request. Not a safety control — the stub has no
// cluster to protect — just a bound that makes a typo visible.
const maxReplicas = 20

func (w *wrenmoor) scaleDeployment(req toolRequest) (any, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	svc, ok := w.services[req.Service]
	if !ok {
		return errorBody("unknown service"), http.StatusNotFound
	}
	if req.Replicas == nil {
		return errorBody("replicas is required"), http.StatusBadRequest
	}
	if *req.Replicas < 0 || *req.Replicas > maxReplicas {
		return errorBody(fmt.Sprintf("replicas must be between 0 and %d", maxReplicas)), http.StatusBadRequest
	}
	previous := svc.Replicas
	svc.Replicas = *req.Replicas
	w.logs[req.Service] = append(w.logs[req.Service],
		logLine{At: w.stamp(), Level: "info", Message: fmt.Sprintf("scaled from %d to %d replicas by incident-agent", previous, svc.Replicas)},
	)
	return map[string]any{
		"service":           svc.Name,
		"previous_replicas": previous,
		"replicas":          svc.Replicas,
		"status":            "scaled",
	}, http.StatusOK
}

// ── On-call: irreversible actions ───────────────────────────────────────────

func (w *wrenmoor) deleteNamespace(req toolRequest) (any, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if _, ok := w.namespaces[req.Namespace]; !ok {
		return errorBody("namespace not found"), http.StatusNotFound
	}
	delete(w.namespaces, req.Namespace)
	return map[string]any{
		"namespace": req.Namespace,
		"status":    "deleted",
		"note":      "Finalizers were removed and every object in the namespace, including its volumes, is gone.",
	}, http.StatusOK
}

func (w *wrenmoor) failoverDatabase(req toolRequest) (any, int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	old, ok := w.primaries[req.Cluster]
	if !ok {
		return errorBody("database cluster not found"), http.StatusNotFound
	}
	promoted := req.Cluster + "-1"
	if old == promoted {
		promoted = req.Cluster + "-0"
	}
	w.primaries[req.Cluster] = promoted
	return map[string]any{
		"cluster":     req.Cluster,
		"old_primary": old,
		"new_primary": promoted,
		"status":      "failed_over",
		"note":        "The old primary was fenced; writes it accepted after the last replicated LSN are lost.",
	}, http.StatusOK
}

// ── Grid data ───────────────────────────────────────────────────────────────

func (w *wrenmoor) listFeeders(req toolRequest) (any, int) {
	out := []feeder{}
	for _, f := range feeders {
		if req.Region != "" && f.Region != req.Region {
			continue
		}
		out = append(out, f)
	}
	return map[string]any{"feeders": out}, http.StatusOK
}

// reading is one half-hourly demand reading. MW is a pointer so a missing
// reading is null on the wire rather than zero: zero is a measurement.
type reading struct {
	Start string   `json:"start"`
	MW    *float64 `json:"mw"`
}

type feederDay struct {
	FeederID       string    `json:"feeder_id"`
	Name           string    `json:"name"`
	Date           string    `json:"date"`
	RatingMW       float64   `json:"rating_mw"`
	PeakMW         float64   `json:"peak_mw"`
	PeakAt         string    `json:"peak_at"`
	UtilisationPct float64   `json:"peak_utilisation_pct"`
	EnergyMWh      float64   `json:"energy_mwh"`
	MissingSlots   int       `json:"missing_readings"`
	Readings       []reading `json:"readings,omitempty"`
}

func (w *wrenmoor) getFeederLoad(req toolRequest) (any, int) {
	f, ok := feederByID(req.FeederID)
	if !ok {
		return errorBody("unknown feeder; call list_feeders for the feeder ids"), http.StatusNotFound
	}
	date := req.Date
	if date == "" {
		date = incidentDay
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return errorBody("date must be YYYY-MM-DD"), http.StatusBadRequest
	}
	return dayFor(f, date, true), http.StatusOK
}

func (w *wrenmoor) getConsumption(req toolRequest) (any, int) {
	date := req.Date
	if date == "" {
		date = incidentDay
	}
	if _, err := time.Parse(time.DateOnly, date); err != nil {
		return errorBody("date must be YYYY-MM-DD"), http.StatusBadRequest
	}
	var total, domestic, commercial, industrial float64
	byFeeder := []map[string]any{}
	for _, f := range feeders {
		if req.Region != "" && f.Region != req.Region {
			continue
		}
		d := dayFor(f, date, false)
		total += d.EnergyMWh
		domestic += d.EnergyMWh * f.Mix.Domestic
		commercial += d.EnergyMWh * f.Mix.Commercial
		industrial += d.EnergyMWh * f.Mix.Industrial
		byFeeder = append(byFeeder, map[string]any{"feeder_id": f.ID, "energy_mwh": d.EnergyMWh, "missing_readings": d.MissingSlots})
	}
	if len(byFeeder) == 0 {
		return errorBody("no feeders in that region; regions are Northmoor and Southvale"), http.StatusNotFound
	}
	region := req.Region
	if region == "" {
		region = "all"
	}
	return map[string]any{
		"date":      date,
		"region":    region,
		"total_mwh": round2(total),
		"by_class_mwh": map[string]float64{
			"domestic":   round2(domestic),
			"commercial": round2(commercial),
			"industrial": round2(industrial),
		},
		"by_feeder": byFeeder,
	}, http.StatusOK
}

type daySummary struct {
	Date               string   `json:"date"`
	TotalMWh           float64  `json:"total_mwh"`
	PeakFeeder         string   `json:"peak_feeder"`
	PeakUtilisationPct float64  `json:"peak_utilisation_pct"`
	FeedersOver90Pct   []string `json:"feeders_over_90pct"`
	MissingReadings    int      `json:"missing_readings"`
}

// getDailySummaries is what the scheduled digest walks. It returns every
// complete day AFTER the cursor, up to yesterday in Europe/London, at most a
// week at a time, and the cursor to carry into the next fire.
//
// An empty cursor means "nothing processed yet" and returns yesterday alone:
// a first fire digests one day rather than the dataset's whole history.
func (w *wrenmoor) getDailySummaries(req toolRequest) (any, int) {
	yesterday := w.now().In(london).AddDate(0, 0, -1)
	last := time.Date(yesterday.Year(), yesterday.Month(), yesterday.Day(), 0, 0, 0, 0, time.UTC)

	var first time.Time
	if req.Cursor == "" {
		first = last
	} else {
		c, err := time.Parse(time.DateOnly, req.Cursor)
		if err != nil {
			return errorBody("cursor must be a YYYY-MM-DD date, or empty"), http.StatusBadRequest
		}
		first = c.AddDate(0, 0, 1)
	}

	days := []daySummary{}
	next := req.Cursor
	for d := first; !d.After(last) && len(days) < maxDigestDays; d = d.AddDate(0, 0, 1) {
		date := d.Format(time.DateOnly)
		days = append(days, summarise(date))
		next = date
	}
	return map[string]any{
		"from_cursor": req.Cursor,
		"days":        days,
		"next_cursor": next,
		"caught_up":   next == last.Format(time.DateOnly),
	}, http.StatusOK
}

func summarise(date string) daySummary {
	s := daySummary{Date: date, FeedersOver90Pct: []string{}}
	var total float64
	for _, f := range feeders {
		d := dayFor(f, date, false)
		total += d.EnergyMWh
		s.MissingReadings += d.MissingSlots
		if d.UtilisationPct > s.PeakUtilisationPct {
			s.PeakUtilisationPct = d.UtilisationPct
			s.PeakFeeder = f.ID
		}
		if d.UtilisationPct >= 90 {
			s.FeedersOver90Pct = append(s.FeedersOver90Pct, f.ID)
		}
	}
	sort.Strings(s.FeedersOver90Pct)
	s.TotalMWh = round2(total)
	return s
}

func feederByID(id string) (feeder, bool) {
	for _, f := range feeders {
		if f.ID == id {
			return f, true
		}
	}
	return feeder{}, false
}

// dayFor computes a feeder's day. Pure: the same feeder and date give the same
// numbers on every call, in every pod, in every take.
func dayFor(f feeder, date string, withReadings bool) feederDay {
	d := feederDay{FeederID: f.ID, Name: f.Name, Date: date, RatingMW: f.RatingMW}
	factor := 1.0
	if t, err := time.Parse(time.DateOnly, date); err == nil {
		if wd := t.Weekday(); wd == time.Saturday || wd == time.Sunday {
			factor = 0.9
		}
	}
	var energy float64
	for slot := range loadShape {
		start := fmt.Sprintf("%02d:%02d", slot/2, (slot%2)*30)
		if missing(f.ID, date, slot) {
			d.MissingSlots++
			if withReadings {
				d.Readings = append(d.Readings, reading{Start: start})
			}
			continue
		}
		mw := round2(f.baseMW * loadShape[slot] * factor * (1 + jitter(f.ID, date, slot)))
		energy += mw * 0.5
		if mw > d.PeakMW {
			d.PeakMW = mw
			d.PeakAt = start
		}
		if withReadings {
			v := mw
			d.Readings = append(d.Readings, reading{Start: start, MW: &v})
		}
	}
	d.EnergyMWh = round2(energy)
	d.UtilisationPct = round2(d.PeakMW / f.RatingMW * 100)
	return d
}

func missing(feederID, date string, slot int) bool {
	for _, g := range readingGaps {
		if g.FeederID == feederID && g.Date == date && slot >= g.FirstSlot && slot <= g.LastSlot {
			return true
		}
	}
	return false
}

// jitter is a deterministic ±3% wobble, so a load curve looks measured rather
// than drawn, and is still the same curve every time it is asked for.
func jitter(feederID, date string, slot int) float64 {
	h := fnv.New32a()
	_, _ = fmt.Fprintf(h, "%s|%s|%d", feederID, date, slot)
	return float64(int(h.Sum32()%61)-30) / 1000
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
