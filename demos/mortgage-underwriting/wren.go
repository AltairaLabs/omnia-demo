package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Wren, Wrenmoor Energy's customer assistant: the account system behind its
// tools, and the two routes the customer portal uses beside the conversation.
// Fixtures in wrendata.go.
//
//	tools (called by the runtime)        portal (called by demo-sites)
//	/wren/bill      read_bill            /wren/consent   the customer clicked
//	/wren/usage     read_usage                           "Switch me"
//	/wren/weather   get_weather          /wren/receipts  the tool calls served
//	/wren/quote     quote_tariff                         to this customer
//	/wren/switch    switch_tariff
//
// THE CONSENT GATE, end to end. switch_tariff needs a consent_id, and only a
// person can make one:
//
//  1. Wren calls request_consent, a CLIENT tool. The facade sends it to the
//     browser, which renders the "Needs your OK" card.
//  2. "Switch me": the portal POSTs /wren/consent through demo-sites, which
//     asserts the signed-in customer's id. This records the consent and
//     returns its id, which the browser sends back as the tool result.
//     "Not now": the browser nacks the call, and no consent exists.
//  3. Wren calls switch_tariff quoting the consent_id. The ToolPolicy refuses
//     the call before dispatch if there is no consent_id at all; this handler
//     refuses one that was never issued, was issued to someone else, was for a
//     different change, or has been used.
//
// The model cannot satisfy step 3 by itself: the id is random and exists only
// once a person has clicked. That is why the check lives here, not only in the
// policy — a ToolPolicy sees the call's own arguments, and an argument is
// something the model writes.
//
// The customer is identified by email on a tool call, and by account id on the
// portal's own routes:
//
//   - On a tool call the runtime sends X-Omnia-User-Email as the edge asserted
//     it, but X-Omnia-User-Id is the workspace PSEUDONYM of the user, not the
//     account id (Omnia pseudonymises user ids with the install's key). So the
//     email is what an account system can look a customer up by.
//   - demo-sites calls the consent and receipt routes directly, not through
//     Omnia, and sets X-Omnia-User-Id to the account id itself.
//
// A call with neither (arena, curl) is Amira's: the stub holds one account.

// wrenReceipt is one tool call the account system served.
type wrenReceipt struct {
	Seq     int    `json:"seq"`
	At      string `json:"at"`
	Tool    string `json:"tool"`
	Detail  string `json:"detail"`
	Outcome string `json:"outcome"`
	Session string `json:"session_id,omitempty"`
	user    string
}

type wrenConsent struct {
	ID      string `json:"consent_id"`
	Tariff  string `json:"tariff"`
	From    string `json:"effective_date"`
	At      string `json:"given_at"`
	Session string `json:"session_id,omitempty"`
	user    string
	used    bool
}

// wren holds the mutable account state: the tariff, consents, and receipts.
// In memory, so a pod restart puts Amira back on Moor Steady for the next take.
type wren struct {
	mu       sync.Mutex
	now      func() time.Time
	customer wrenCustomer
	pending  *wrenSwitch
	consents map[string]*wrenConsent
	receipts []wrenReceipt
	newID    func() string
}

// wrenSwitch is a scheduled tariff change.
type wrenSwitch struct {
	Reference string `json:"reference"`
	From      string `json:"from_tariff"`
	To        string `json:"to_tariff"`
	Effective string `json:"effective_date"`
	ConsentID string `json:"consent_id"`
}

// maxReceipts bounds the receipt log; the oldest fall off.
const maxReceipts = 500

func newWren(now func() time.Time) *wren {
	return &wren{
		now:      now,
		customer: seedWrenCustomer(),
		consents: map[string]*wrenConsent{},
		newID: func() string {
			b := make([]byte, 6)
			_, _ = rand.Read(b)
			return hex.EncodeToString(b)
		},
	}
}

// wrenRequest is the union of the Wren tools' arguments.
type wrenRequest struct {
	Period        string `json:"period"`
	Days          int    `json:"days"`
	Granularity   string `json:"granularity"`
	Date          string `json:"date"`
	Postcode      string `json:"postcode"`
	From          string `json:"from"`
	To            string `json:"to"`
	Tariff        string `json:"tariff"`
	EffectiveDate string `json:"effective_date"`
	ConsentID     string `json:"consent_id"`
	SessionID     string `json:"session_id"`
}

// wrenCall is one request as a handler sees it.
type wrenCall struct {
	req     wrenRequest
	user    string
	session string
}

func (w *wren) register(mux *http.ServeMux) {
	mux.HandleFunc("/wren/bill", w.tool("read_bill", "read", w.readBill))
	mux.HandleFunc("/wren/usage", w.tool("read_usage", "read", w.readUsage))
	mux.HandleFunc("/wren/weather", w.tool("get_weather", "read", w.getWeather))
	mux.HandleFunc("/wren/quote", w.tool("quote_tariff", "read", w.quoteTariff))
	mux.HandleFunc("/wren/switch", w.tool("switch_tariff", "write", w.switchTariff))
	mux.HandleFunc("/wren/consent", w.giveConsent)
	mux.HandleFunc("/wren/receipts", w.listReceipts)
}

// tool wraps a Wren tool: POST, decode, identify the customer, run, record the
// receipt, encode. The receipt is recorded whatever the outcome, so a refused
// switch shows as refused rather than as nothing.
func (w *wren) tool(name, effect string, fn func(wrenCall) (any, string, int)) http.HandlerFunc {
	return func(rw http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req wrenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && !errors.Is(err, io.EOF) {
			http.Error(rw, "invalid request body", http.StatusBadRequest)
			return
		}
		c := wrenCall{req: req, user: userOf(r), session: r.Header.Get("X-Omnia-Session-Id")}
		log.Printf("tool request received: tool=%s ref=%s remote=%s", name, c.user, r.RemoteAddr)

		var body any
		var detail string
		status := http.StatusOK
		if c.user != wrenAccountID {
			body, status = errorBody("no account for this customer"), http.StatusNotFound
		} else {
			body, detail, status = fn(c)
		}

		outcome := effect + " · allowed"
		if effect == "write" {
			outcome = "write · done"
		}
		if status >= 400 {
			outcome = effect + " · refused"
		}
		w.record(name, detail, outcome, c)

		rw.Header().Set("Content-Type", "application/json")
		rw.WriteHeader(status)
		if err := json.NewEncoder(rw).Encode(body); err != nil {
			log.Printf("encode failed: tool=%s err=%v", name, err)
		}
	}
}

func userOf(r *http.Request) string {
	if e := strings.TrimSpace(r.Header.Get("X-Omnia-User-Email")); e != "" {
		if strings.EqualFold(e, wrenEmail) {
			return wrenAccountID
		}
		return "email:" + e // someone the stub holds no account for
	}
	if u := strings.TrimSpace(r.Header.Get("X-Omnia-User-Id")); u != "" {
		return u
	}
	return wrenAccountID
}

func (w *wren) record(tool, detail, outcome string, c wrenCall) {
	w.mu.Lock()
	defer w.mu.Unlock()
	seq := 1
	if n := len(w.receipts); n > 0 {
		seq = w.receipts[n-1].Seq + 1
	}
	w.receipts = append(w.receipts, wrenReceipt{
		Seq: seq, At: w.now().UTC().Format(time.RFC3339), Tool: tool, Detail: detail,
		Outcome: outcome, Session: c.session, user: c.user,
	})
	if len(w.receipts) > maxReceipts {
		w.receipts = w.receipts[len(w.receipts)-maxReceipts:]
	}
}

// ── Reads ───────────────────────────────────────────────────────────────────

func (w *wren) readBill(c wrenCall) (any, string, int) {
	period := c.req.Period
	if period == "" {
		period = "2026-10"
	}
	b, ok := wrenBills[period]
	if !ok {
		return errorBody("no bill for that period; bills exist for 2026-09 and 2026-10"), period, http.StatusNotFound
	}
	out := map[string]any{"bill": b}
	if prev, ok := wrenBills[previousPeriod(period)]; ok {
		out["previous"] = map[string]any{
			"period": prev.Period, "kwh": prev.KWh, "total_gbp": prev.TotalPounds,
			"change_gbp": round2(b.TotalPounds - prev.TotalPounds),
			"change_kwh": round2(b.KWh - prev.KWh),
		}
	}
	w.mu.Lock()
	out["customer"] = w.customer
	w.mu.Unlock()
	return out, period, http.StatusOK
}

func previousPeriod(p string) string {
	t, err := time.Parse("2006-01", p)
	if err != nil {
		return ""
	}
	return t.AddDate(0, -1, 0).Format("2006-01")
}

type usageDay struct {
	Date         string    `json:"date"`
	KWh          float64   `json:"kwh"`
	OvernightKWh float64   `json:"overnight_kwh"`
	Partial      bool      `json:"partial,omitempty"`
	HalfHourly   []float64 `json:"half_hourly_kwh,omitempty"`
}

// readUsage reads the half-hourly meter data. It always reads half-hourly;
// what it returns is a day-by-day summary (total, and the overnight
// 00:00–07:00 share the storage heaters draw), plus the 48 readings themselves
// when two days or fewer are asked for.
func (w *wren) readUsage(c wrenCall) (any, string, int) {
	days := c.req.Days
	if days <= 0 || days > wrenUsageDays {
		days = wrenUsageDays
	}
	start := wrenUsageDays - days
	first, _ := time.Parse(time.DateOnly, wrenUsageStart)
	withSlots := days <= 2

	out := make([]usageDay, 0, days)
	monthly := map[string]float64{}
	var total, overnight float64
	for i := start; i < wrenUsageDays; i++ {
		date := first.AddDate(0, 0, i).Format(time.DateOnly)
		slots := daySlots(i)
		d := usageDay{Date: date, KWh: wrenDaily[i], Partial: date == wrenToday}
		for s, v := range slots {
			if s < 14 {
				d.OvernightKWh += v
			}
		}
		d.OvernightKWh = round2(d.OvernightKWh)
		if withSlots {
			d.HalfHourly = slots
		}
		out = append(out, d)
		total += d.KWh
		overnight += d.OvernightKWh
		monthly[date[:7]] += d.KWh
	}
	for k, v := range monthly {
		monthly[k] = round2(v)
	}
	detail := fmt.Sprintf("half-hourly, %d days", days)
	return map[string]any{
		"meter":               "Smart, half-hourly",
		"from":                out[0].Date,
		"to":                  out[len(out)-1].Date,
		"days":                out,
		"total_kwh":           round2(total),
		"overnight_share_pct": round2(overnight / total * 100),
		"kwh_by_month":        monthly,
		"note":                "Today is read to 14:00 only. Month totals here cover only the days returned; the bill holds the whole month.",
	}, detail, http.StatusOK
}

// daySlots spreads day i's kWh across its half-hours by usageShape. Today
// stops at 14:00.
func daySlots(i int) []float64 {
	slots := daySlotsExact(i)
	for s := range slots {
		slots[s] = math.Round(slots[s]*1000) / 1000
	}
	return slots
}

func daySlotsExact(i int) []float64 {
	n := 48
	if i == wrenUsageDays-1 {
		n = wrenTodaySlots
	}
	var weight float64
	for s := 0; s < n; s++ {
		weight += usageShape[s]
	}
	slots := make([]float64, n)
	for s := 0; s < n; s++ {
		slots[s] = wrenDaily[i] * usageShape[s] / weight
	}
	return slots
}

type weatherDay struct {
	Date         string  `json:"date"`
	OvernightMin float64 `json:"overnight_min_c"`
	WindMph      int     `json:"wind_mph_on_wrenmoor_edge"`
}

const wrenMildMin = 9.0

func (w *wren) getWeather(c wrenCall) (any, string, int) {
	pc := strings.ToUpper(strings.TrimSpace(c.req.Postcode))
	if pc == "" {
		pc = "HX7"
	}
	if !strings.HasPrefix(pc, "HX7") {
		return errorBody("weather is held for Wrenmoor Energy's area only (postcode district HX7)"), pc, http.StatusNotFound
	}
	from, to := c.req.From, c.req.To
	if from == "" {
		from = "2026-10-01"
	}
	if to == "" {
		to = wrenToday
	}
	f, err1 := time.Parse(time.DateOnly, from)
	t, err2 := time.Parse(time.DateOnly, to)
	if err1 != nil || err2 != nil || t.Before(f) {
		return errorBody("from and to must be YYYY-MM-DD, from not after to"), pc, http.StatusBadRequest
	}
	if t.Sub(f) > 92*24*time.Hour {
		return errorBody("at most 92 days at a time"), pc, http.StatusBadRequest
	}
	days := []weatherDay{}
	for d := f; !d.After(t); d = d.AddDate(0, 0, 1) {
		date := d.Format(time.DateOnly)
		low := wrenMildMin + float64(d.Day()%4) - 1
		if cold, ok := wrenColdNights[date]; ok {
			low = cold
		}
		days = append(days, weatherDay{Date: date, OvernightMin: low, WindMph: 14 + (d.Day()*7)%19})
	}
	if days[len(days)-1].Date == wrenToday {
		days[len(days)-1].WindMph = 31
	}
	return map[string]any{"postcode_district": "HX7", "days": days}, pc[:3], http.StatusOK
}

// quoteTariff prices a tariff against the customer's own October half-hourly
// usage, so the comparison is like for like with the October bill.
func (w *wren) quoteTariff(c wrenCall) (any, string, int) {
	id := c.req.Tariff
	if id == "" {
		id = "moor-flex"
	}
	t, ok := wrenTariffs[id]
	if !ok {
		return errorBody("unknown tariff; tariffs are moor-steady and moor-flex"), id, http.StatusNotFound
	}
	oct := wrenBills["2026-10"]
	credit := oct.Lines[2].Pounds
	standing := round2(float64(oct.Days) * t.StandingP / 100)

	var energyP float64
	first, _ := time.Parse(time.DateOnly, wrenUsageStart)
	for i := 0; i < wrenUsageDays; i++ {
		if first.AddDate(0, 0, i).Format("2006-01") != "2026-10" {
			continue
		}
		// Unrounded, so Moor Steady re-prices October to the penny.
		for s, kwh := range daySlotsExact(i) {
			if id == "moor-flex" {
				energyP += kwh * flexPriceP(s)
			} else {
				energyP += kwh * wrenSteadyRateP
			}
		}
	}
	energy := round2(energyP / 100)
	estimate := round2(energy + standing + credit)

	w.mu.Lock()
	current := w.customer.Tariff
	w.mu.Unlock()
	return map[string]any{
		"tariff":             t,
		"current_tariff":     current,
		"based_on":           "Your October 2026 half-hourly usage (312.4 kWh)",
		"october_actual_gbp": oct.TotalPounds,
		"october_estimate": map[string]any{
			"energy_gbp": energy, "standing_gbp": standing, "credit_gbp": credit, "total_gbp": estimate,
			"average_unit_rate_p": round2(energyP / oct.KWh),
		},
		"saving_vs_actual_gbp": round2(oct.TotalPounds - estimate),
		"earliest_switch_date": wrenSwitchDate,
		"exit_fee_gbp":         0,
		"switch_back_within":   "14 days, at no cost",
	}, id, http.StatusOK
}

// ── The write ───────────────────────────────────────────────────────────────

// switchTariff schedules the change, and only on a consent the customer gave.
func (w *wren) switchTariff(c wrenCall) (any, string, int) {
	r := c.req
	detail := r.Tariff
	if r.EffectiveDate != "" {
		detail += " from " + r.EffectiveDate
	}
	if _, ok := wrenTariffs[r.Tariff]; !ok {
		return errorBody("unknown tariff; tariffs are moor-steady and moor-flex"), detail, http.StatusBadRequest
	}
	if r.EffectiveDate == "" {
		r.EffectiveDate = wrenSwitchDate
	}

	w.mu.Lock()
	defer w.mu.Unlock()
	if r.ConsentID == "" {
		return errorBody("no consent_id: ask the customer with request_consent first"), detail, http.StatusForbidden
	}
	cons, ok := w.consents[r.ConsentID]
	switch {
	case !ok || cons.user != c.user:
		return errorBody("that consent_id was not given by this customer"), detail, http.StatusForbidden
	case cons.used:
		return errorBody("that consent has already been used"), detail, http.StatusConflict
	case cons.Tariff != r.Tariff || cons.From != r.EffectiveDate:
		return errorBody(fmt.Sprintf("the customer agreed to %s from %s, not %s from %s; ask again for this change",
			cons.Tariff, cons.From, r.Tariff, r.EffectiveDate)), detail, http.StatusForbidden
	}
	if w.customer.Tariff == r.Tariff {
		return errorBody("the customer is already on that tariff"), detail, http.StatusConflict
	}

	cons.used = true
	w.pending = &wrenSwitch{
		Reference: "SW-" + strings.ToUpper(w.newID()[:8]),
		From:      w.customer.Tariff, To: r.Tariff, Effective: r.EffectiveDate, ConsentID: cons.ID,
	}
	w.customer.Tariff = r.Tariff
	return map[string]any{
		"status":         "scheduled",
		"switch":         w.pending,
		"how_to_reverse": "Switch back to " + wrenTariffs[w.pending.From].Name + " within 14 days at no cost: ask Wren or call us.",
	}, detail, http.StatusOK
}

// ── Portal routes ───────────────────────────────────────────────────────────

// giveConsent records the customer's click on "Switch me". It is NOT a tool:
// the model has no route to it. demo-sites calls it with the signed-in
// customer's id.
func (w *wren) giveConsent(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req wrenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(rw, "invalid request body", http.StatusBadRequest)
		return
	}
	user := userOf(r)
	rw.Header().Set("Content-Type", "application/json")
	if user != wrenAccountID {
		rw.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(rw).Encode(errorBody("no account for this customer"))
		return
	}
	if _, ok := wrenTariffs[req.Tariff]; !ok {
		rw.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(rw).Encode(errorBody("unknown tariff"))
		return
	}
	if req.EffectiveDate == "" {
		req.EffectiveDate = wrenSwitchDate
	}
	w.mu.Lock()
	cons := &wrenConsent{
		ID: "CNS-" + strings.ToUpper(w.newID()), Tariff: req.Tariff, From: req.EffectiveDate,
		At: w.now().UTC().Format(time.RFC3339), Session: req.SessionID, user: user,
	}
	w.consents[cons.ID] = cons
	w.mu.Unlock()
	log.Printf("consent given: consent=%s tariff=%s from=%s", cons.ID, cons.Tariff, cons.From)
	_ = json.NewEncoder(rw).Encode(cons)
}

// listReceipts returns this customer's tool calls after ?after=<seq>, and for
// ?session=<id> when the calls carried one. Calls with no session (arena,
// curl) are included, so a local run shows its receipts too.
func (w *wren) listReceipts(rw http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(rw, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	user := userOf(r)
	session := r.URL.Query().Get("session")
	after, _ := strconv.Atoi(r.URL.Query().Get("after"))

	w.mu.Lock()
	out := []wrenReceipt{}
	last := 0
	for _, rc := range w.receipts {
		last = rc.Seq
		if rc.Seq <= after || rc.user != user {
			continue
		}
		if session != "" && rc.Session != "" && rc.Session != session {
			continue
		}
		out = append(out, rc)
	}
	w.mu.Unlock()
	rw.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(rw).Encode(map[string]any{"receipts": out, "last_seq": last})
}
