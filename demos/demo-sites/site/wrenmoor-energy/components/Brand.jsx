import React from "react";

// Wrenmoor Energy's shared components. Styles live in styles.css; these keep
// the markup the three UI kits share in one place.

// The logo, built in CSS: a moor circle with a sun dot at its top right, then
// the word mark. There is no logo file.
export function Logo({ size = 26, word = "Wrenmoor", wordSize = 24, color = "var(--peat)", disc = "var(--moor)", href }) {
  const dot = Math.round(size * 0.42);
  const mark = (
    <span style={{ display: "flex", alignItems: "center", gap: 10, color }}>
      <span aria-hidden="true" style={{
        width: size, height: size, borderRadius: "50%", background: disc, flex: "none",
        display: "flex", alignItems: "flex-start", justifyContent: "flex-end",
      }}>
        <span style={{ width: dot, height: dot, borderRadius: "50%", background: "var(--sun)", margin: "2px 2px 0 0" }} />
      </span>
      <span style={{ fontFamily: "var(--font-serif)", fontSize: wordSize, fontWeight: 500, letterSpacing: "-0.01em" }}>{word}</span>
    </span>
  );
  return href ? <a href={href} aria-label={word} style={{ color }}>{mark}</a> : mark;
}

// The demo disclaimer. Every page carries it, word for word.
export function Disclaimer({ style }) {
  return (
    <div className="disclaimer" style={style}>
      Wrenmoor Energy is a fictional company created for demonstration purposes by{" "}
      <a href="https://altairalabs.ai">AltairaLabs.ai</a>. Names, data and people shown are not real.
    </div>
  );
}

export function Badge({ tone = "neutral", dot, children, style }) {
  return (
    <span className={"badge badge-" + tone} style={style}>
      {dot ? <span className="dot" style={{ width: 7, height: 7, background: dot }} /> : null}
      {children}
    </span>
  );
}

export function LiveDot({ color = "var(--live)", size = 8 }) {
  return <span className="dot" style={{ width: size, height: size, background: color }} />;
}

// EnergyMix: a pill bar split by flex weight, 3px gaps.
export function MixBar({ parts, height = 12 }) {
  return (
    <div className="mix-bar" style={{ height }} role="img"
      aria-label={parts.map((p) => p.label + " " + p.pct + "%").join(", ")}>
      {parts.map((p) => <div key={p.key} style={{ flex: p.pct, background: p.color }} />)}
    </div>
  );
}

export function MixLegend({ parts }) {
  return (
    <div className="mix-legend">
      {parts.map((p) => (
        <span key={p.key}><span className="dot" style={{ width: 9, height: 9, background: p.color }} />{p.label} {p.pct}%</span>
      ))}
    </div>
  );
}

// UsageBars: only the bars the copy talks about are moor; partial days hatched.
export function UsageBars({ days, scale }) {
  return (
    <div className="usage" role="img" aria-label={days.map((d) => d.date + " " + d.kwh + " kWh").join(", ")}>
      {days.map((d) => (
        <div key={d.date} title={d.date + " · " + d.kwh + " kWh" + (d.partial ? " so far" : "")}
          className={d.highlight ? "on" : d.partial ? "partial" : ""}
          style={{ height: Math.round((d.kwh / scale) * 100) + "%" }} />
      ))}
    </div>
  );
}

// ── The agent pattern ──────────────────────────────────────────────────────

export function AgentAvatar({ large }) {
  return <div className={"agent-avatar" + (large ? " lg" : "")} aria-hidden="true">W</div>;
}

export function UserBubble({ children }) {
  return <div className="user-bubble">{children}</div>;
}

// AgentReceipt: what the agent read, shown whenever it read data.
export function AgentReceipt({ items }) {
  if (!items || items.length === 0) return null;
  return (
    <div className="receipt" aria-label="What Wren checked">
      {items.map((r, i) => (
        <span key={i} style={r.refused ? { color: "var(--clay-ink)" } : null}>
          {r.refused ? "✕" : "✓"} {r.tool}{r.detail ? " · " + r.detail : ""}
        </span>
      ))}
    </div>
  );
}

// AgentConfirm: "Needs your OK". Any write or contract change goes through it.
export function AgentConfirm({ text, yes = "Switch me", no = "Not now", onYes, onNo, state, busy }) {
  return (
    <div className="confirm">
      <span style={{ fontSize: 14, fontWeight: 600 }}>Needs your OK</span>
      <span style={{ fontSize: 14, lineHeight: 1.5 }}>{text}</span>
      {state ? (
        <span style={{ fontSize: 14, fontWeight: 500, color: state === "yes" ? "var(--moor)" : "var(--peat-muted)" }}>
          {state === "yes" ? "You said yes." : state === "no" ? "You said not now. Nothing changed." : state}
        </span>
      ) : (
        <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
          <button className="btn btn-primary btn-sm" disabled={busy} onClick={onYes}>{yes}</button>
          <button className="btn btn-outline btn-sm" disabled={busy} onClick={onNo}>{no}</button>
        </div>
      )}
    </div>
  );
}
