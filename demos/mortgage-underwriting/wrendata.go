package main

// Synthetic data for Wren, Wrenmoor Energy's customer assistant. The figures
// are the ones the customer portal shows (demos/demo-sites/site/
// wrenmoor-energy/data/account.js); change one and change the other.
//
// The story they tell, which the agent should be able to find for itself:
//
//   - Amira's October bill is £84.10, up £14.20 on September, because she used
//     38 kWh more: 312.4 kWh against 274.4. Most of the extra is 20–24 October,
//     when it was 3°C overnight and her storage heaters ran longer. Her unit
//     rate did not change.
//   - Her heaters mostly run overnight, when Wrenmoor Edge makes the most spare
//     power, so Moor Flex would have cost about £75 for the same October.
//   - Another cold snap on 5–6 November shows in the last 14 days of usage,
//     and belongs to November's bill, not October's.
//
// Everything is fixed: no clock, no randomness. "Today" is the portal's
// Monday 9 November, with readings up to 14:00.

const (
	wrenToday       = "2026-11-09"
	wrenTodaySlots  = 28 // half-hours read today: 00:00 to 14:00
	wrenUsageStart  = "2026-09-11"
	wrenUsageDays   = 60
	wrenSwitchDate  = "2026-12-01"
	wrenAccountID   = "40021187"
	wrenStandingP   = 46.0
	wrenSteadyRateP = 24.1
)

// wrenCustomer is the one account the stub holds.
type wrenCustomer struct {
	AccountID string `json:"account_id"`
	Name      string `json:"name"`
	Address   string `json:"address"`
	Postcode  string `json:"postcode"`
	Tariff    string `json:"tariff"`
	MeterType string `json:"meter"`
	Heating   string `json:"heating"`
}

func seedWrenCustomer() wrenCustomer {
	return wrenCustomer{
		AccountID: wrenAccountID,
		Name:      "Amira Saleem",
		Address:   "12 Birchcliffe Rd, Hebden Bridge",
		Postcode:  "HX7 8DJ",
		Tariff:    "moor-steady",
		MeterType: "Smart, half-hourly",
		Heating:   "Electric storage heaters",
	}
}

type billLine struct {
	Label  string  `json:"label"`
	Detail string  `json:"detail"`
	Pounds float64 `json:"amount_gbp"`
}

type bill struct {
	Period      string     `json:"period"`
	Status      string     `json:"status"`
	Due         string     `json:"due"`
	Tariff      string     `json:"tariff"`
	UnitRateP   float64    `json:"unit_rate_p_per_kwh"`
	StandingP   float64    `json:"standing_charge_p_per_day"`
	KWh         float64    `json:"kwh"`
	Days        int        `json:"days"`
	Lines       []billLine `json:"lines"`
	TotalPounds float64    `json:"total_gbp"`
}

// wrenBills holds the two bills Wren compares. 312.4 × 24.1p = £75.29;
// 31 × 46p = £14.26; less £5.45 credit = £84.10. September: 274.4 kWh,
// £66.13 + £13.80 − £10.03 credit = £69.90, so October is up £14.20.
var wrenBills = map[string]bill{
	"2026-10": {
		Period: "2026-10", Status: "due", Due: "2026-11-15", Tariff: "moor-steady",
		UnitRateP: wrenSteadyRateP, StandingP: wrenStandingP, KWh: 312.4, Days: 31,
		Lines: []billLine{
			{Label: "Electricity", Detail: "312.4 kWh at 24.1p", Pounds: 75.29},
			{Label: "Standing charge", Detail: "31 days at 46p", Pounds: 14.26},
			{Label: "Credit from September", Detail: "", Pounds: -5.45},
		},
		TotalPounds: 84.10,
	},
	"2026-09": {
		Period: "2026-09", Status: "paid", Due: "2026-10-15", Tariff: "moor-steady",
		UnitRateP: wrenSteadyRateP, StandingP: wrenStandingP, KWh: 274.4, Days: 30,
		Lines: []billLine{
			{Label: "Electricity", Detail: "274.4 kWh at 24.1p", Pounds: 66.13},
			{Label: "Standing charge", Detail: "30 days at 46p", Pounds: 13.80},
			{Label: "Credit from August", Detail: "", Pounds: -10.03},
		},
		TotalPounds: 69.90,
	},
}

// wrenDaily is kWh per day from wrenUsageStart, 60 days, oldest first. The
// last entry is today, read to 14:00 only.
//
// 11–30 Sep, then October (summing to 312.4, with the cold spell on 20–24),
// then 1–9 November (the portal's 14-day chart is the last 14 of these).
var wrenDaily = [wrenUsageDays]float64{
	// September 11–30
	9.0, 9.2, 8.8, 9.1, 9.4, 9.3, 8.9, 9.0, 9.2, 8.7,
	9.1, 9.3, 9.0, 8.9, 9.4, 9.2, 9.0, 9.1, 9.3, 9.2,
	// October 1–19
	8.3, 8.9, 9.2, 8.8, 9.1, 9.6, 9.3, 8.7, 8.9, 9.2,
	8.8, 9.1, 9.6, 9.3, 8.7, 8.9, 9.2, 8.8, 9.1,
	// October 20–24: the cold spell
	13.8, 15.9, 16.4, 14.6, 12.5,
	// October 25–31
	9.1, 9.0, 8.8, 10.0, 9.4, 11.0, 10.4,
	// November 1–9 (5–6: the second cold snap; 9th: today, to 14:00)
	13.6, 12.4, 11.6, 12.2, 19.4, 17.6, 14.4, 11.8, 8.0,
}

// wrenColdNights are the overnight minimums that explain the usage. Every
// other night is mild (wrenMildMin).
var wrenColdNights = map[string]float64{
	"2026-10-20": 4, "2026-10-21": 3, "2026-10-22": 3, "2026-10-23": 3, "2026-10-24": 5,
	"2026-11-05": 2, "2026-11-06": 3,
}

// usageShape spreads a day's kWh across its 48 half-hours: storage heaters
// charging overnight (00:00–07:00), a flat day, an evening peak.
var usageShape = func() [48]float64 {
	var s [48]float64
	for i := range s {
		switch {
		case i < 14:
			s[i] = 3.0
		case i < 16:
			s[i] = 1.2
		case i < 34:
			s[i] = 0.9
		case i < 44:
			s[i] = 1.6
		default:
			s[i] = 1.0
		}
	}
	return s
}()

// flexPriceP is Moor Flex's average price per half-hour slot over October:
// cheapest overnight, when Wrenmoor Edge makes the most spare power, dearest
// in the evening peak. Rates on the day run 14–29p.
func flexPriceP(slot int) float64 {
	switch {
	case slot < 14:
		return 16.8
	case slot < 34:
		return 24.0
	case slot < 44:
		return 29.0
	default:
		return 21.0
	}
}

type wrenTariff struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Kind      string  `json:"kind"`
	UnitRate  string  `json:"unit_rate"`
	StandingP float64 `json:"standing_charge_p_per_day"`
	Summary   string  `json:"summary"`
}

var wrenTariffs = map[string]wrenTariff{
	"moor-steady": {
		ID: "moor-steady", Name: "Moor Steady", Kind: "Fixed 12 months", UnitRate: "24.1p/kWh, every hour",
		StandingP: wrenStandingP, Summary: "One price for every hour of the day.",
	},
	"moor-flex": {
		ID: "moor-flex", Name: "Moor Flex", Kind: "Cheaper when it's windy", UnitRate: "14–29p/kWh, by half-hour",
		StandingP: wrenStandingP, Summary: "The price drops when Wrenmoor's turbines make more than customers need. Best hours are texted the night before.",
	},
}
