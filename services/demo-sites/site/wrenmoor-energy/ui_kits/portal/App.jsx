import React from "react";
import { Logo, Disclaimer, Badge, LiveDot, MixBar, UsageBars } from "../../components/Brand.jsx";
import { CUSTOMER, BILL, USAGE_14, USAGE_SCALE, MIX } from "../../data/account.js";
import { WrenPanel } from "./Wren.jsx";

// Wrenmoor Energy's customer portal (the handoff's "Wrenmoor Customer
// Portal"): Amira's account overview, with Wren in the side panel. The figures
// come from data/account.js, which matches what Wren reads from the account
// system.

const PHOTOS = "../../assets/photos/";

function Header() {
  const tab = (label, on) => (
    <a key={label} href="#" onClick={(e) => e.preventDefault()} className={on ? "" : "navlink"}
      aria-current={on ? "page" : undefined}
      style={on
        ? { padding: "8px 14px", borderRadius: "var(--r-pill)", background: "var(--moor-tint)", color: "var(--moor-tint-ink)", fontWeight: 500 }
        : { padding: "8px 14px", color: "var(--peat-body)" }}>{label}</a>
  );
  return (
    <header style={{ borderBottom: "1px solid var(--line)", background: "var(--card)" }}>
      <div style={{ maxWidth: 1440, margin: "0 auto", padding: "14px 28px", display: "flex", alignItems: "center", justifyContent: "space-between", gap: 20, flexWrap: "wrap" }}>
        <div style={{ display: "flex", alignItems: "center", gap: 36, flexWrap: "wrap" }}>
          <Logo href="../marketing/" size={24} wordSize={22} />
          <nav style={{ display: "flex", gap: 4, fontSize: 15, flexWrap: "wrap" }} aria-label="Account">
            {tab("Overview", true)}{tab("Usage")}{tab("Bills")}{tab("Tariff")}{tab("Meter")}
          </nav>
        </div>
        <div style={{ display: "flex", alignItems: "center", gap: 12, fontSize: 14 }}>
          <span className="muted">{CUSTOMER.address}</span>
          <div aria-label={"Signed in as " + CUSTOMER.firstName} style={{ width: 34, height: 34, borderRadius: "50%", background: "var(--sun-tint)", display: "flex", alignItems: "center", justifyContent: "center", fontWeight: 600, fontSize: 13, color: "var(--sun-ink)" }}>{CUSTOMER.initials}</div>
        </div>
      </div>
    </header>
  );
}

function PowerNow() {
  return (
    <div className="card card-lg" style={{ padding: 32, display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,260px),1fr))", gap: 32, alignItems: "center" }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 18 }}>
        <div role="img" aria-label="A wind turbine above low cloud" style={{ height: 140, borderRadius: "var(--r-tile)", background: "var(--photo-bg) url('" + PHOTOS + "portal-power-now.jpg') center/cover no-repeat" }} />
        <span className="muted" style={{ fontSize: 15, display: "flex", alignItems: "center", gap: 8 }}><LiveDot />Your power right now</span>
        <span className="serif" style={{ fontSize: "clamp(40px,5vw,60px)", lineHeight: 0.95, letterSpacing: "-0.02em" }}>Mostly wind from Wrenmoor Edge</span>
        <span className="body-copy" style={{ fontSize: 15, lineHeight: 1.5, maxWidth: "40ch" }}>It's blowing 31 mph up on the Edge. Prices on Moor Flex would be 16p/kWh until about 7pm.</span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 16 }}>
        <MixBar parts={MIX.parts} height={16} />
        <div style={{ display: "flex", flexDirection: "column", gap: 10, fontSize: 15 }}>
          {MIX.parts.map((p, i) => (
            <div key={p.key} style={{ display: "flex", justifyContent: "space-between", gap: 12, paddingBottom: i < MIX.parts.length - 1 ? 10 : 0, borderBottom: i < MIX.parts.length - 1 ? "1px solid var(--line-soft)" : "none" }}>
              <span style={{ display: "flex", gap: 10, alignItems: "center" }}><span className="dot" style={{ width: 10, height: 10, background: p.color }} />{p.label} · {p.site}</span>
              <span className="mono" style={{ fontSize: 14 }}>{p.pct}%</span>
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}

function Bill() {
  return (
    <div className="card" style={{ padding: 26, display: "flex", flexDirection: "column", gap: 18 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12 }}>
        <span style={{ fontSize: 17, fontWeight: 600 }}>{BILL.title}</span>
        <Badge tone="due" style={{ padding: "5px 11px" }}>{BILL.due}</Badge>
      </div>
      <div style={{ display: "flex", alignItems: "baseline", gap: 10, flexWrap: "wrap" }}>
        <span className="big" style={{ fontSize: 52 }}>{BILL.total}</span>
        <span style={{ fontSize: 14, color: "var(--clay-ink)" }}>{BILL.change}</span>
      </div>
      <div className="body-copy" style={{ display: "flex", flexDirection: "column", gap: 8, fontSize: 14 }}>
        {BILL.lines.map(([label, amount]) => (
          <div key={label} style={{ display: "flex", justifyContent: "space-between" }}><span>{label}</span><span className="mono" style={{ fontSize: 13 }}>{amount}</span></div>
        ))}
      </div>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
        <button className="btn btn-primary" style={{ padding: "11px 20px" }}>Pay {BILL.total}</button>
        <button className="btn btn-outline" style={{ padding: "10px 19px" }}>View statement</button>
      </div>
    </div>
  );
}

function Usage() {
  return (
    <div className="card" style={{ padding: 26, display: "flex", flexDirection: "column", gap: 18 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: 12 }}>
        <span style={{ fontSize: 17, fontWeight: 600 }}>Last 14 days</span>
        <span className="mono muted" style={{ fontSize: 13 }}>kWh/day</span>
      </div>
      <UsageBars days={USAGE_14} scale={USAGE_SCALE} />
      <div className="mono muted" style={{ display: "flex", justifyContent: "space-between", fontSize: 11 }}><span>27 Oct</span><span>cold snap</span><span>today</span></div>
      <span className="body-copy" style={{ fontSize: 14, lineHeight: 1.5 }}>You used about 11 kWh a day. The two cold days at the start of the month used nearly twice that.</span>
    </div>
  );
}

function TariffNudge() {
  return (
    <div style={{ background: "var(--wind-tint)", borderRadius: "var(--r-card)", padding: "24px 26px", display: "flex", justifyContent: "space-between", alignItems: "center", gap: 20, flexWrap: "wrap" }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 4, maxWidth: "60ch" }}>
        <span style={{ fontSize: 16, fontWeight: 600 }}>You're on Moor Steady · 24.1p/kWh</span>
        <span style={{ fontSize: 15, lineHeight: 1.5, color: "var(--wind-body)" }}>Based on how you use power, Moor Flex would have cost about £9 less last month.</span>
      </div>
      <a className="btn btn-card" href="../marketing/#tariffs" style={{ padding: "11px 20px" }}>Compare tariffs</a>
    </div>
  );
}

function App() {
  return (
    <div style={{ minHeight: "100vh", display: "flex", flexDirection: "column" }}>
      <Header />
      <div style={{ flex: 1, maxWidth: 1440, width: "100%", margin: "0 auto", padding: "36px 28px 64px", display: "flex", flexWrap: "wrap", gap: 28, alignItems: "flex-start" }}>
        <main style={{ flex: "2 1 600px", minWidth: 0, display: "flex", flexDirection: "column", gap: 24 }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            <span className="muted" style={{ fontSize: 15 }}>{CUSTOMER.dateLine}</span>
            <h1 className="h1">Good afternoon, {CUSTOMER.firstName}.</h1>
          </div>
          <PowerNow />
          <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,280px),1fr))", gap: 24 }}>
            <Bill />
            <Usage />
          </div>
          <TariffNudge />
        </main>
        <WrenPanel />
      </div>
      <Disclaimer />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
