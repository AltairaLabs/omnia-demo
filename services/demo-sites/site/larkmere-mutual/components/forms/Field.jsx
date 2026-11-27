import React from "react";

export function Field({ label, htmlFor, hint, error, required = false, children, style }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-3)", ...style }}>
      {label ? (
        <label htmlFor={htmlFor} style={{ font: "var(--type-label)", color: "var(--text-secondary)" }}>
          {label}
          {required ? <span style={{ color: "var(--text-negative)" }}> *</span> : null}
        </label>
      ) : null}
      {children}
      {error ? (
        <div style={{ font: "var(--type-body-s)", color: "var(--text-negative)" }}>{error}</div>
      ) : hint ? (
        <div style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>{hint}</div>
      ) : null}
    </div>
  );
}
