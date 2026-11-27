import React from "react";
import { IconButton } from "../core/IconButton.jsx";

export function Dialog({ open, onClose, title, description, children, footer, width = 520 }) {
  React.useEffect(() => {
    if (!open) return;
    const onKey = (e) => e.key === "Escape" && onClose && onClose();
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [open, onClose]);
  if (!open) return null;
  return (
    <div
      onClick={onClose}
      style={{
        position: "fixed", inset: 0, background: "var(--surface-scrim)", zIndex: 60,
        display: "flex", alignItems: "center", justifyContent: "center", padding: "var(--space-7)",
        animation: "none",
      }}
    >
      <div
        role="dialog"
        aria-modal="true"
        onClick={(e) => e.stopPropagation()}
        style={{
          width: "100%", maxWidth: width, background: "var(--surface-card)",
          border: "1px solid var(--border-subtle)", borderRadius: "var(--radius-xl)",
          boxShadow: "var(--shadow-3)", padding: "var(--space-8)", boxSizing: "border-box",
        }}
      >
        <div style={{ display: "flex", alignItems: "flex-start", justifyContent: "space-between", gap: "var(--space-5)" }}>
          <h2 style={{ font: "var(--type-heading-m)", letterSpacing: "var(--tracking-tight)" }}>{title}</h2>
          {onClose ? <IconButton icon="x" label="Close" size="sm" onClick={onClose} /> : null}
        </div>
        {description ? (
          <p style={{ font: "var(--type-body)", color: "var(--text-secondary)", margin: "var(--space-4) 0 0" }}>{description}</p>
        ) : null}
        {children ? <div style={{ marginTop: "var(--space-6)" }}>{children}</div> : null}
        {footer ? (
          <div style={{ display: "flex", justifyContent: "flex-end", gap: "var(--space-4)", marginTop: "var(--space-7)", paddingTop: "var(--space-6)", borderTop: "1px solid var(--border-subtle)" }}>
            {footer}
          </div>
        ) : null}
      </div>
    </div>
  );
}
