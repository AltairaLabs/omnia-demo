import React from "react";
import { Disclaimer, Badge, LiveDot } from "../../components/Brand.jsx";

// Wrenmoor Hub, the staff intranet (the handoff's "Wrenmoor Intranet").
// SAMPLE DATA, shaped on the agents this chart actually deploys for Wrenmoor
// Energy: Wren, the on-call incident agent, the field-service copilot and the
// grid analyst. The live record is in the Omnia dashboard on this same host;
// those links are ROOT-relative (/agents, /sessions) on purpose, so they leave
// the /wrenmoor-energy/ prefix the site is served under and land on the
// dashboard. A proxy must not rewrite them.

const WORKSPACE = "wrenmoor-energy";
const DASH_AGENTS = "/agents?workspace=" + WORKSPACE;
const DASH_SESSIONS = "/sessions?workspace=" + WORKSPACE;

const AGENTS = [
  { name: "Wren · customer assistant", where: "Portal, app, website chat", status: "Live", dot: "var(--live)", model: "gpt-4o", version: "wren@1.0.0", sessions: "1,962", evals: "48 / 50", highlight: true },
  { name: "On-call incident agent", where: "Platform team", status: "Investigating", dot: "var(--sun)", model: "gpt-4o", version: "incident-agent@1.0.0", sessions: "3", evals: "15 / 15" },
  { name: "Field-service copilot", where: "Field engineers, manuals in memory", status: "Live", dot: "var(--live)", model: "gpt-4o", version: "field-service@1.0.0", sessions: "141", evals: "22 / 22" },
  { name: "Grid analyst", where: "06:00 digest, and A2A for on-call", status: "Scheduled", dot: "var(--idle)", model: "gpt-4o", version: "data-analyst@1.0.0", sessions: "78", evals: "12 / 12", muted: true },
];

const SESSION = [
  { at: "14:18:02", tool: "read_bill", args: "period=2026-10", verdict: "read · allowed" },
  { at: "14:18:03", tool: "read_usage", args: "days=60", verdict: "read · allowed" },
  { at: "14:18:03", tool: "get_weather", args: "postcode=HX7", verdict: "read · allowed" },
  { at: "14:18:05", tool: "quote_tariff", args: "tariff=moor-flex", verdict: "read · allowed" },
  { at: "14:18:07", tool: "request_consent", args: "tariff=moor-flex from=2026-12-01", verdict: "client · waiting", held: true },
  { at: "", tool: "switch_tariff", args: "tariff=moor-flex from=2026-12-01", verdict: "write · held", held: true },
];

const INCIDENT = [
  ["08:14", <>meter-ingest 5xx rate went to 18.4% three minutes after deploy <span className="mono" style={{ fontSize: 12 }}>meter-ingest:v2.14.0</span>. Export readings from Wrenmoor Edge and Cragg Vale Solar were being rejected.</>],
  ["08:19", <>Asked the grid analyst, over A2A, about output at the two sites. Generation was normal: the gap is missing readings, not lost power.</>],
  ["08:22", <>Rolled back to v2.13.2. Reversible, so allowed by policy. Errors back under 2% by 08:25.</>],
  ["08:47", <>Ignored an instruction found in a billing-api log line asking it to scale payments to 0. Flagged for review.</>],
  ["08:52", <>Asked to delete the stuck <span className="mono" style={{ fontSize: 12 }}>outage-map-staging</span> namespace. <b style={{ fontWeight: 600, color: "var(--clay-ink)" }}>Denied</b>: irreversible actions need a human (WE-OPS-07). It suggested scaling outage-map to 0 so the volume releases.</>],
];

function Sidebar() {
  const nav = ["Home", "Customers", "Generation", "Agents", "Incidents", "People"];
  return (
    <aside style={{ background: "var(--peat)", color: "var(--disclaimer-ink)", padding: "22px 14px", display: "flex", flexDirection: "column", gap: 28 }}>
      <a href="../marketing/" style={{ display: "flex", alignItems: "center", gap: 10, padding: "0 8px", color: "var(--footer-head)" }}>
        <span aria-hidden="true" style={{ width: 24, height: 24, borderRadius: "50%", background: "oklch(0.55 0.08 165)", display: "flex", alignItems: "flex-start", justifyContent: "flex-end", flex: "none" }}>
          <span style={{ width: 10, height: 10, borderRadius: "50%", background: "var(--sun)", margin: "2px 2px 0 0" }} />
        </span>
        <span className="serif" style={{ fontSize: 21, fontWeight: 500 }}>Wrenmoor Hub</span>
      </a>
      <nav style={{ display: "flex", flexDirection: "column", gap: 2, fontSize: 14 }} aria-label="Hub">
        {nav.map((n) => (
          <a key={n} href={n === "Agents" ? "#agents" : "#"} aria-current={n === "Agents" ? "page" : undefined}
            style={n === "Agents"
              ? { padding: "9px 12px", borderRadius: 10, background: "var(--peat-raised)", color: "var(--footer-head)", fontWeight: 500 }
              : { padding: "9px 12px", borderRadius: 10, color: "inherit" }}>{n}</a>
        ))}
      </nav>
      <div style={{ marginTop: "auto", padding: "14px 12px", borderRadius: 12, background: "var(--peat-panel)", display: "flex", flexDirection: "column", gap: 6, fontSize: 12 }}>
        <span style={{ color: "var(--footer-head)", fontWeight: 500 }}>Agents run on Omnia</span>
        <span className="mono" style={{ fontSize: 11 }}>workspace {WORKSPACE}</span>
        <a href={DASH_AGENTS} style={{ color: "var(--footer-head)", textDecoration: "underline", textUnderlineOffset: 3 }}>Open the dashboard</a>
      </div>
    </aside>
  );
}

function Tile({ label, value }) {
  return (
    <div className="card" style={{ borderRadius: "var(--r-tile)", padding: "18px 20px", display: "flex", flexDirection: "column", gap: 6 }}>
      <span className="muted" style={{ fontSize: 13 }}>{label}</span>
      <span className="big" style={{ fontSize: 36 }}>{value}</span>
    </div>
  );
}

const cols = "2.2fr 1fr 1.2fr 1.3fr 0.8fr 0.9fr";

function Registry() {
  return (
    <div id="agents" className="card" style={{ overflowX: "auto" }}>
      <div style={{ minWidth: 760, display: "flex", flexDirection: "column", fontSize: 14 }}>
        <div className="muted" style={{ display: "grid", gridTemplateColumns: cols, gap: 16, padding: "14px 22px", borderBottom: "1px solid oklch(0.90 0.012 85)", fontSize: 12 }}>
          <span>Agent</span><span>Status</span><span>Model</span><span>Version</span><span>Sessions</span><span>Eval pass</span>
        </div>
        {AGENTS.map((a, i) => (
          // Clicking an agent opens its sessions, in the live dashboard.
          <a key={a.name} href={DASH_SESSIONS} title="Open this agent's sessions in the Omnia dashboard"
            style={{ display: "grid", gridTemplateColumns: cols, gap: 16, padding: "16px 22px", alignItems: "center", color: "var(--peat)",
              borderBottom: i < AGENTS.length - 1 ? "1px solid var(--line-faint)" : "none", background: a.highlight ? "var(--moor-row)" : "transparent" }}>
            <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
              <span style={{ fontWeight: 600 }}>{a.name}</span>
              <span className="muted" style={{ fontSize: 12 }}>{a.where}</span>
            </div>
            <span style={{ display: "flex", gap: 6, alignItems: "center", color: a.muted ? "var(--peat-muted)" : "inherit" }}><LiveDot color={a.dot} size={7} />{a.status}</span>
            <span className="mono" style={{ fontSize: 12 }}>{a.model}</span>
            <span className="mono" style={{ fontSize: 12 }}>{a.version}</span>
            <span className="mono" style={{ fontSize: 12 }}>{a.sessions}</span>
            <span className="mono" style={{ fontSize: 12 }}>{a.evals}</span>
          </a>
        ))}
      </div>
    </div>
  );
}

function SessionRecord() {
  return (
    <div className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <span style={{ fontSize: 17, fontWeight: 600 }}>Session · Amira S., account 40021187</span>
          <span className="mono muted" style={{ fontSize: 12 }}>ses_7f3a91 · wren@1.0.0 · gpt-4o · 14:18</span>
        </div>
        <Badge tone="due" style={{ padding: "5px 11px" }}>Waiting on customer</Badge>
      </div>
      <div className="mono" style={{ display: "flex", flexDirection: "column", fontSize: 12 }}>
        {SESSION.map((s, i) => (
          <div key={s.tool} style={{ display: "grid", gridTemplateColumns: "52px 1fr auto", gap: 12, padding: "10px 0", borderBottom: i < SESSION.length - 1 ? "1px solid var(--line-faint)" : "none" }}>
            <span style={{ color: "var(--peat-faint)" }}>{s.at || "—"}</span>
            <span>{s.tool} <span style={{ color: "var(--peat-faint)" }}>{s.args}</span></span>
            <span style={{ color: s.held ? "var(--sun-ink)" : "var(--moor)" }}>{s.verdict}</span>
          </div>
        ))}
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 6, padding: "14px 16px", borderRadius: "var(--r-input)", background: "var(--confirm-bg)", border: "1px solid var(--confirm-line)", fontSize: 13, lineHeight: 1.5 }}>
        <span style={{ fontWeight: 600 }}>Policy decision · wren-policy</span>
        <span>switch_tariff changes a contract, so it needs an explicit yes from the account holder. It is refused without a consent_id, and only the account holder's "Switch me" in the portal creates one. Held until they confirm.</span>
      </div>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
        <a className="btn btn-primary btn-sm" href={DASH_SESSIONS}>Open session record</a>
      </div>
    </div>
  );
}

function Incident() {
  return (
    <div className="card" style={{ padding: 24, display: "flex", flexDirection: "column", gap: 18 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-start", gap: 12, flexWrap: "wrap" }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
          <span style={{ fontSize: 17, fontWeight: 600 }}>Incident · meter-ingest 5xx</span>
          <span className="mono muted" style={{ fontSize: 12 }}>ALT-7731 · incident-agent@1.0.0 · opened 08:14</span>
        </div>
        <Badge tone="paid" style={{ padding: "5px 11px" }}>Mitigated</Badge>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 14, fontSize: 14, lineHeight: 1.5 }}>
        {INCIDENT.map(([at, text]) => (
          <div key={at} style={{ display: "flex", gap: 10 }}>
            <span className="mono" style={{ flex: "none", fontSize: 12, color: "var(--peat-faint)", width: 44, paddingTop: 2 }}>{at}</span>
            <span>{text}</span>
          </div>
        ))}
      </div>
      <div style={{ display: "flex", gap: 10, flexWrap: "wrap" }}>
        <a className="btn btn-primary btn-sm" href={DASH_SESSIONS}>Open session record</a>
        <button className="btn btn-outline btn-sm">Scale outage-map to 0</button>
      </div>
    </div>
  );
}

function App() {
  return (
    <div style={{ minHeight: "100vh", display: "grid", gridTemplateColumns: "minmax(0,232px) minmax(0,1fr)", background: "var(--paper-sunken)" }}>
      <Sidebar />
      <main style={{ minWidth: 0, padding: "28px 32px 64px", display: "flex", flexDirection: "column", gap: 24 }}>
        <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end", gap: 20, flexWrap: "wrap" }}>
          <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
            <span className="muted" style={{ fontSize: 14 }}>Monday 9 November · 14:22</span>
            <h1 className="serif" style={{ fontWeight: 400, fontSize: 40, lineHeight: 1.05, margin: 0, letterSpacing: "-0.015em" }}>Agents</h1>
          </div>
          <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
            <span className="muted" style={{ fontSize: 14 }}>Signed in as Dan Pickles, Platform</span>
            <div style={{ width: 32, height: 32, borderRadius: "50%", background: "var(--moor-tint)", display: "flex", alignItems: "center", justifyContent: "center", fontWeight: 600, fontSize: 12, color: "oklch(0.35 0.06 165)" }}>DP</div>
          </div>
        </div>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(170px,1fr))", gap: 16 }}>
          <Tile label="Customer sessions today" value="2,184" />
          <Tile label="Waiting on a customer's OK" value="37" />
          <Tile label="Blocked by policy" value="4" />
          <Tile label="Handed to a person" value="6.1%" />
        </div>
        <Registry />
        <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,420px),1fr))", gap: 24, alignItems: "start" }}>
          <SessionRecord />
          <Incident />
        </div>
      </main>
      <Disclaimer style={{ gridColumn: "1 / -1" }} />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
