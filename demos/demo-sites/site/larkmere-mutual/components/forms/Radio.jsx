import React from "react";

export function Radio({ name, value, checked = false, onChange, label, description, disabled = false, id, style }) {
  const [hover, setHover] = React.useState(false);
  return (
    <label htmlFor={id}
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{ display: "flex", gap: "var(--space-4)", alignItems: "flex-start", cursor: disabled ? "not-allowed" : "pointer", minHeight: 24, ...style }}>
      <input id={id} type="radio" name={name} value={value} checked={checked} onChange={onChange} disabled={disabled}
        style={{ position: "absolute", opacity: 0, width: 0, height: 0 }} />
      <span style={{
        width: 20, height: 20, flex: "0 0 auto", marginTop: 1, borderRadius: "var(--radius-pill)",
        display: "inline-flex", alignItems: "center", justifyContent: "center",
        background: disabled ? "var(--ink-100)" : "var(--surface-card)",
        border: "1.5px solid " + (checked ? "var(--green-800)" : hover && !disabled ? "var(--border-strong)" : "var(--border-default)"),
        transition: "var(--transition-control)",
      }}>
        {checked ? <span style={{ width: 10, height: 10, borderRadius: "var(--radius-pill)", background: "var(--green-800)" }} /> : null}
      </span>
      <span>
        <span style={{ font: "var(--type-body)", color: disabled ? "var(--ink-400)" : "var(--text-primary)" }}>{label}</span>
        {description ? <span style={{ display: "block", font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: 2 }}>{description}</span> : null}
      </span>
    </label>
  );
}

export function RadioGroup({ children, style }) {
  return <div role="radiogroup" style={{ display: "flex", flexDirection: "column", gap: "var(--space-4)", ...style }}>{children}</div>;
}
