// Sample data the Wrenmoor Energy pages render. Static, because the pages
// are; Wren reads the same figures LIVE from the account system's tools, so
// they are kept equal to demos/mortgage-underwriting/wrendata.go. Change one
// and change the other, or Wren will contradict the page it sits on.

export const CUSTOMER = {
  firstName: "Amira",
  initials: "AS",
  account: "40021187",
  address: "12 Birchcliffe Rd, HX7 8DJ",
  dateLine: "Monday 9 November",
};

// October's bill: 312.4 kWh x 24.1p + 31 days x 46p - £5.45 credit.
export const BILL = {
  title: "October bill",
  total: "£84.10",
  due: "Due in 6 days",
  change: "+£14.20 on September",
  lines: [
    ["Electricity · 312.4 kWh", "£75.29"],
    ["Standing charge · 31 days", "£14.26"],
    ["Credit from September", "−£5.45"],
  ],
};

// The last 14 days, kWh/day, 27 Oct to today. 5 and 6 Nov are the cold snap,
// the only bars the copy talks about, so the only ones drawn in moor. Today is
// read to 14:00, so it is partial (hatched). Heights are kWh / USAGE_SCALE.
export const USAGE_SCALE = 20;
export const USAGE_14 = [
  { date: "27 Oct", kwh: 8.8 }, { date: "28 Oct", kwh: 10.0 }, { date: "29 Oct", kwh: 9.4 },
  { date: "30 Oct", kwh: 11.0 }, { date: "31 Oct", kwh: 10.4 }, { date: "1 Nov", kwh: 13.6 },
  { date: "2 Nov", kwh: 12.4 }, { date: "3 Nov", kwh: 11.6 }, { date: "4 Nov", kwh: 12.2 },
  { date: "5 Nov", kwh: 19.4, highlight: true }, { date: "6 Nov", kwh: 17.6, highlight: true },
  { date: "7 Nov", kwh: 14.4 }, { date: "8 Nov", kwh: 11.8 }, { date: "9 Nov", kwh: 8.0, partial: true },
];

// The live generation mix: wind, solar, battery. Colours by source; battery is
// moor, never sun or wind.
export const MIX = {
  at: "14:20",
  totalMW: "41.6 MW",
  parts: [
    { key: "wind", label: "Wind", site: "Wrenmoor Edge", pct: 64, color: "var(--wind)" },
    { key: "solar", label: "Solar", site: "Cragg Vale", pct: 21, color: "var(--sun)" },
    { key: "battery", label: "Battery", site: "Heptonstall", pct: 15, color: "var(--moor)" },
  ],
};

export const TARIFFS = {
  standing: "46p/day standing",
  steady: { name: "Moor Steady", rate: "24.1p", badge: "Fixed 12 months" },
  flex: { name: "Moor Flex", rate: "14–29p", badge: "Cheaper when it's windy" },
};

export const SITES = [
  { name: "Wrenmoor Edge", detail: "14 turbines · 29 MW · since 2019", photo: "site-wrenmoor-edge.jpg" },
  { name: "Cragg Vale Solar", detail: "42,000 panels · 18 MW · since 2022", photo: "site-cragg-vale-solar.jpg" },
  { name: "Heptonstall Store", detail: "Battery · 40 MWh · keeps the lights on when it's still", photo: "site-heptonstall-store.jpg" },
];
