import React from "react";

const TONES = {
  neutral: { bg: "var(--sand-100)", fg: "var(--ink-700)", bd: "var(--ink-200)" },
  positive: { bg: "var(--positive-100)", fg: "var(--positive-700)", bd: "var(--green-200)" },
  caution: { bg: "var(--caution-100)", fg: "var(--caution-700)", bd: "#EAD6AE" },
  negative: { bg: "var(--negative-100)", fg: "var(--negative-700)", bd: "#F0CCC5" },
  info: { bg: "var(--info-100)", fg: "var(--info-700)", bd: "#C9DCEC" },
  accent: { bg: "var(--mint-100)", fg: "var(--mint-600)", bd: "var(--mint-200)" },
};

export function Badge({ children, tone = "neutral", dot = false, style }) {
  const t = TONES[tone] || TONES.neutral;
  return (
    <span style={{
      display: "inline-flex", alignItems: "center", gap: "var(--space-2)",
      background: t.bg, color: t.fg, border: "1px solid " + t.bd,
      borderRadius: "var(--radius-pill)", padding: "3px 10px",
      font: "var(--type-label)", fontSize: 13, whiteSpace: "nowrap",
      ...style,
    }}>
      {dot ? <span style={{ width: 6, height: 6, borderRadius: "var(--radius-pill)", background: t.fg }} /> : null}
      {children}
    </span>
  );
}
