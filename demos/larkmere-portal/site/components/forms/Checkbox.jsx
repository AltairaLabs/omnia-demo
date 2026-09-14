import React from "react";
import { Icon } from "../core/Icon.jsx";

export function Checkbox({ checked = false, onChange, label, description, disabled = false, id, style }) {
  const [hover, setHover] = React.useState(false);
  return (
    <label
      htmlFor={id}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{ display: "flex", gap: "var(--space-4)", alignItems: "flex-start", cursor: disabled ? "not-allowed" : "pointer", minHeight: 24, ...style }}
    >
      <input id={id} type="checkbox" checked={checked} onChange={onChange} disabled={disabled}
        style={{ position: "absolute", opacity: 0, width: 0, height: 0 }} />
      <span style={{
        width: 20, height: 20, flex: "0 0 auto", marginTop: 1, display: "inline-flex", alignItems: "center", justifyContent: "center",
        borderRadius: "var(--radius-xs)",
        background: disabled ? "var(--ink-100)" : checked ? "var(--green-800)" : "var(--surface-card)",
        border: "1px solid " + (checked ? "var(--green-800)" : hover && !disabled ? "var(--border-strong)" : "var(--border-default)"),
        color: "var(--mint-400)", transition: "var(--transition-control)",
      }}>
        {checked ? <Icon name="check" size={14} strokeWidth={2} /> : null}
      </span>
      <span>
        <span style={{ font: "var(--type-body)", color: disabled ? "var(--ink-400)" : "var(--text-primary)" }}>{label}</span>
        {description ? <span style={{ display: "block", font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: 2 }}>{description}</span> : null}
      </span>
    </label>
  );
}
