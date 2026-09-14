import React from "react";

export function Switch({ checked = false, onChange, label, description, disabled = false, id, style }) {
  return (
    <label htmlFor={id} style={{ display: "flex", gap: "var(--space-5)", alignItems: "flex-start", justifyContent: "space-between", cursor: disabled ? "not-allowed" : "pointer", ...style }}>
      <span>
        <span style={{ font: "var(--type-body)", color: disabled ? "var(--ink-400)" : "var(--text-primary)" }}>{label}</span>
        {description ? <span style={{ display: "block", font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: 2 }}>{description}</span> : null}
      </span>
      <input id={id} type="checkbox" role="switch" checked={checked} onChange={onChange} disabled={disabled}
        style={{ position: "absolute", opacity: 0, width: 0, height: 0 }} />
      <span style={{
        width: 46, height: 28, flex: "0 0 auto", borderRadius: "var(--radius-pill)", position: "relative",
        background: disabled ? "var(--ink-100)" : checked ? "var(--mint-500)" : "var(--ink-200)",
        transition: "background-color var(--dur-base) var(--ease-standard)",
      }}>
        <span style={{
          position: "absolute", top: 3, left: checked ? 21 : 3, width: 22, height: 22,
          borderRadius: "var(--radius-pill)", background: "var(--white)", boxShadow: "var(--shadow-1)",
          transition: "left var(--dur-base) var(--ease-standard)",
        }} />
      </span>
    </label>
  );
}
