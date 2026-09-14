import React from "react";
import { Icon } from "../core/Icon.jsx";
import { IconButton } from "../core/IconButton.jsx";

const TONES = {
  positive: { icon: "circle-check", fg: "var(--positive-700)", bd: "var(--green-200)", bg: "var(--positive-100)" },
  caution: { icon: "triangle-alert", fg: "var(--caution-700)", bd: "#EAD6AE", bg: "var(--caution-100)" },
  negative: { icon: "circle-alert", fg: "var(--negative-700)", bd: "#F0CCC5", bg: "var(--negative-100)" },
  info: { icon: "info", fg: "var(--info-700)", bd: "#C9DCEC", bg: "var(--info-100)" },
};

export function Toast({ tone = "positive", title, description, action, onDismiss, style }) {
  const t = TONES[tone] || TONES.positive;
  return (
    <div role="status" style={{
      display: "flex", gap: "var(--space-4)", alignItems: "flex-start",
      background: t.bg, border: "1px solid " + t.bd, borderRadius: "var(--radius-lg)",
      boxShadow: "var(--shadow-2)", padding: "var(--space-5)", maxWidth: 460, boxSizing: "border-box", ...style,
    }}>
      <span style={{ color: t.fg, display: "inline-flex", marginTop: 1 }}><Icon name={t.icon} size={20} /></span>
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ font: "var(--type-heading-s)", color: "var(--text-primary)" }}>{title}</div>
        {description ? <div style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", marginTop: 2 }}>{description}</div> : null}
        {action ? <div style={{ marginTop: "var(--space-4)" }}>{action}</div> : null}
      </div>
      {onDismiss ? <IconButton icon="x" label="Dismiss" size="sm" onClick={onDismiss} /> : null}
    </div>
  );
}
