package main

import (
	"fmt"
	"hash/fnv"
	"math"
	"net/http"
	"time"
)

// Wrenmoor Energy's generation data, for the grid analyst: the three sites the
// public website shows, their half-hourly output, and the live energy mix.
//
//	WM-EDGE   Wrenmoor Edge       wind     14 turbines, 29 MW
//	WM-CRAGG  Cragg Vale Solar    solar    42,000 panels, 18 MW
//	WM-HEPT   Heptonstall Store   battery  40 MWh, 20 MW
//
// EVERYTHING IS A PURE FUNCTION of the site, the date and the half-hour, so the
// same question gets the same figures on every take, in every pod. The one
// pinned point is 14:00-14:30 on the incident day, which is the website's live
// card ("Live from our sites · 14:20", 41.6 MW: wind 64%, solar 21%, battery
// 15%). The incident's other trace is here too: meter-ingest rev 42 rejected
// the export-meter readings from Wrenmoor Edge and Cragg Vale Solar from 08:30
// to 11:30, so those half-hours are MISSING, not zero.

// site is one generation site.
type site struct {
	ID          string  `json:"site_id"`
	Name        string  `json:"name"`
	Kind        string  `json:"kind"`
	CapacityMW  float64 `json:"capacity_mw"`
	StorageMWh  float64 `json:"storage_mwh,omitempty"`
	Units       string  `json:"units"`
	Location    string  `json:"location"`
	Since       int     `json:"commissioned"`
	ExportMeter string  `json:"export_meter"`
}

const (
	kindWind    = "wind"
	kindSolar   = "solar"
	kindBattery = "battery"
)

// sites is ordered, so listings are stable.
var sites = []site{
	{ID: "WM-EDGE", Name: "Wrenmoor Edge", Kind: kindWind, CapacityMW: 29, Units: "14 turbines", Location: "Calderdale", Since: 2019, ExportMeter: "HE-EDGE-2"},
	{ID: "WM-CRAGG", Name: "Cragg Vale Solar", Kind: kindSolar, CapacityMW: 18, Units: "42,000 panels", Location: "Calderdale", Since: 2022, ExportMeter: "HE-EDGE-2"},
	{ID: "WM-HEPT", Name: "Heptonstall Store", Kind: kindBattery, CapacityMW: 20, StorageMWh: 40, Units: "battery", Location: "Calderdale", Since: 2024, ExportMeter: "HE-HEPT-1"},
}

func siteByID(id string) (site, bool) {
	for _, s := range sites {
		if s.ID == id {
			return s, true
		}
	}
	return site{}, false
}

// liveSlot is 14:00-14:30, the half-hour the website's "14:20" card reads.
const liveSlot = 28

// The website's live card, which the incident day reproduces at liveSlot.
const (
	liveWindMW    = 26.6
	liveSolarMW   = 8.7
	liveBatteryMW = 6.3
)

// readingGap is a stretch of missing half-hourly export readings. Slot
// indices, inclusive.
type readingGap struct {
	Date      string
	SiteID    string
	FirstSlot int
	LastSlot  int
}

// From 08:30 on the incident day, the slot after rev 42 started rejecting
// batches, to 11:30. The analyst reports it as missing data, not as a drop in
// output: zero is a reading, and the turbines kept turning.
var readingGaps = []readingGap{
	{Date: incidentDay, SiteID: "WM-EDGE", FirstSlot: 17, LastSlot: 23},
	{Date: incidentDay, SiteID: "WM-CRAGG", FirstSlot: 17, LastSlot: 23},
}

func missing(siteID, date string, slot int) bool {
	for _, g := range readingGaps {
		if g.SiteID == siteID && g.Date == date && slot >= g.FirstSlot && slot <= g.LastSlot {
			return true
		}
	}
	return false
}

// unit is a deterministic number in [0, 1) for a key.
func unit(parts ...any) float64 {
	h := fnv.New32a()
	_, _ = fmt.Fprint(h, parts...)
	return float64(h.Sum32()%10000) / 10000
}

// windiness is a day's wind as a fraction of Wrenmoor Edge's capacity at
// liveSlot. The incident day is pinned to the live card.
func windiness(date string) float64 {
	if date == incidentDay {
		return liveWindMW / 29
	}
	return 0.2 + 0.65*unit("wind", date)
}

// windShape is a gentle swing through the day, 1.0 at liveSlot.
func windShape(slot int) float64 {
	return 1 + 0.12*math.Sin(float64(slot-liveSlot)*math.Pi/24)
}

// daylight is the solar bell for a November day: 07:30 to 16:00.
func daylight(slot int) float64 {
	const first, last = 15, 32
	if slot < first || slot >= last {
		return 0
	}
	return math.Sin(math.Pi * float64(slot-first) / float64(last-first))
}

// clearness is a day's sky, pinned on the incident day to the live card.
func clearness(date string) float64 {
	if date == incidentDay {
		return liveSolarMW / (18 * daylight(liveSlot))
	}
	return 0.25 + 0.6*unit("sun", date)
}

// batteryMW is the store's schedule: charge overnight on cheap wind (negative),
// discharge in the afternoon and the evening peak (positive). Round trip about
// 90%: it charges 40 MWh and returns about 36.
func batteryMW(slot int) float64 {
	switch {
	case slot >= 2 && slot <= 11:
		return -8.0
	case slot >= 26 && slot <= 29:
		return liveBatteryMW
	case slot >= 34 && slot <= 39:
		return 7.8
	default:
		return 0
	}
}

// outputMW is one site's output in one half-hour.
func outputMW(s site, date string, slot int) float64 {
	switch s.Kind {
	case kindWind:
		return round2(math.Min(s.CapacityMW, s.CapacityMW*windiness(date)*windShape(slot)))
	case kindSolar:
		return round2(s.CapacityMW * clearness(date) * daylight(slot))
	default:
		return batteryMW(slot)
	}
}

// reading is one half-hourly output reading. MW is a pointer so a missing
// reading is null on the wire rather than zero: zero is a measurement.
type reading struct {
	Start string   `json:"start"`
	MW    *float64 `json:"mw"`
}

type siteDay struct {
	SiteID          string    `json:"site_id"`
	Name            string    `json:"name"`
	Kind            string    `json:"kind"`
	Date            string    `json:"date"`
	CapacityMW      float64   `json:"capacity_mw"`
	PeakMW          float64   `json:"peak_mw"`
	PeakAt          string    `json:"peak_at"`
	EnergyMWh       float64   `json:"energy_mwh"`
	CapacityFactor  float64   `json:"capacity_factor_pct"`
	ChargedMWh      float64   `json:"charged_mwh,omitempty"`
	MissingReadings int       `json:"missing_readings"`
	Readings        []reading `json:"readings,omitempty"`
}

// dayFor computes a site's day. Energy is what was exported (for the battery,
// what it discharged; ChargedMWh is what it took in). Capacity factor is over
// the half-hours actually read.
func dayFor(s site, date string, withReadings bool) siteDay {
	d := siteDay{SiteID: s.ID, Name: s.Name, Kind: s.Kind, Date: date, CapacityMW: s.CapacityMW}
	var energy, charged float64
	read := 0
	for slot := 0; slot < 48; slot++ {
		start := fmt.Sprintf("%02d:%02d", slot/2, (slot%2)*30)
		if missing(s.ID, date, slot) {
			d.MissingReadings++
			if withReadings {
				d.Readings = append(d.Readings, reading{Start: start})
			}
			continue
		}
		read++
		mw := outputMW(s, date, slot)
		if mw >= 0 {
			energy += mw * 0.5
		} else {
			charged += -mw * 0.5
		}
		if mw > d.PeakMW {
			d.PeakMW, d.PeakAt = mw, start
		}
		if withReadings {
			v := mw
			d.Readings = append(d.Readings, reading{Start: start, MW: &v})
		}
	}
	d.EnergyMWh = round2(energy)
	d.ChargedMWh = round2(charged)
	if read > 0 {
		d.CapacityFactor = round2(energy / (s.CapacityMW * 0.5 * float64(read)) * 100)
	}
	return d
}

// ── Handlers ────────────────────────────────────────────────────────────────

func (w *wrenmoor) listSites(_ toolRequest) (any, int) {
	return map[string]any{"sites": sites}, http.StatusOK
}

func validDate(date string) (string, bool) {
	if date == "" {
		return incidentDay, true
	}
	_, err := time.Parse(time.DateOnly, date)
	return date, err == nil
}

func (w *wrenmoor) getSiteOutput(req toolRequest) (any, int) {
	s, ok := siteByID(req.SiteID)
	if !ok {
		return errorBody("unknown site; call list_sites for the site ids (WM-EDGE, WM-CRAGG, WM-HEPT)"), http.StatusNotFound
	}
	date, ok := validDate(req.Date)
	if !ok {
		return errorBody("date must be YYYY-MM-DD"), http.StatusBadRequest
	}
	return dayFor(s, date, true), http.StatusOK
}

type mixPart struct {
	Source string  `json:"source"`
	Site   string  `json:"site"`
	Value  float64 `json:"value"`
	Pct    float64 `json:"pct"`
}

// getEnergyMix is the live mix (no date: the half-hour the website's card
// reads, 14:00 on the incident day) or a day's energy by source.
func (w *wrenmoor) getEnergyMix(req toolRequest) (any, int) {
	date, ok := validDate(req.Date)
	if !ok {
		return errorBody("date must be YYYY-MM-DD"), http.StatusBadRequest
	}
	if req.Date == "" {
		parts, total := mixAt(incidentDay, liveSlot)
		return map[string]any{
			"live": true, "at": incidentDay + "T14:20:00Z", "total_mw": total, "unit": "MW", "parts": parts,
		}, http.StatusOK
	}
	parts := []mixPart{}
	var total float64
	for _, s := range sites {
		d := dayFor(s, date, false)
		parts = append(parts, mixPart{Source: s.Kind, Site: s.Name, Value: d.EnergyMWh})
		total += d.EnergyMWh
	}
	for i := range parts {
		parts[i].Pct = round2(parts[i].Value / total * 100)
	}
	return map[string]any{"live": false, "date": date, "total_mwh": round2(total), "unit": "MWh", "parts": parts}, http.StatusOK
}

func mixAt(date string, slot int) ([]mixPart, float64) {
	parts := []mixPart{}
	var total float64
	for _, s := range sites {
		mw := math.Max(0, outputMW(s, date, slot))
		parts = append(parts, mixPart{Source: s.Kind, Site: s.Name, Value: mw})
		total += mw
	}
	for i := range parts {
		parts[i].Pct = math.Round(parts[i].Value / total * 100)
	}
	return parts, round2(total)
}

type daySummary struct {
	Date              string             `json:"date"`
	GenerationMWh     float64            `json:"generation_mwh"`
	BySourceMWh       map[string]float64 `json:"by_source_mwh"`
	MixPct            map[string]float64 `json:"mix_pct"`
	WindCFPct         float64            `json:"wind_capacity_factor_pct"`
	SolarCFPct        float64            `json:"solar_capacity_factor_pct"`
	BatteryChargedMWh float64            `json:"battery_charged_mwh"`
	MissingReadings   int                `json:"missing_readings"`
	SitesWithGaps     []string           `json:"sites_with_gaps"`
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
	s := daySummary{Date: date, BySourceMWh: map[string]float64{}, MixPct: map[string]float64{}, SitesWithGaps: []string{}}
	var total float64
	for _, st := range sites {
		d := dayFor(st, date, false)
		s.BySourceMWh[st.Kind] = d.EnergyMWh
		total += d.EnergyMWh
		s.MissingReadings += d.MissingReadings
		if d.MissingReadings > 0 {
			s.SitesWithGaps = append(s.SitesWithGaps, st.ID)
		}
		switch st.Kind {
		case kindWind:
			s.WindCFPct = d.CapacityFactor
		case kindSolar:
			s.SolarCFPct = d.CapacityFactor
		case kindBattery:
			s.BatteryChargedMWh = d.ChargedMWh
		}
	}
	for k, v := range s.BySourceMWh {
		s.MixPct[k] = round2(v / total * 100)
	}
	s.GenerationMWh = round2(total)
	return s
}

func round2(v float64) float64 {
	return math.Round(v*100) / 100
}
