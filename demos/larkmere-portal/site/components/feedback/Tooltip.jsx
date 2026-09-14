import React from "react";

export function Tooltip({ label, placement = "top", children, style }) {
  const [open, setOpen] = React.useState(false);
  const pos = {
    top: { bottom: "calc(100% + 6px)", left: "50%", transform: "translateX(-50%)" },
    bottom: { top: "calc(100% + 6px)", left: "50%", transform: "translateX(-50%)" },
    left: { right: "calc(100% + 6px)", top: "50%", transform: "translateY(-50%)" },
    right: { left: "calc(100% + 6px)", top: "50%", transform: "translateY(-50%)" },
  }[placement];
  return (
    <span
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onFocus={() => setOpen(true)}
      onBlur={() => setOpen(false)}
      style={{ position: "relative", display: "inline-flex", ...style }}
    >
      {children}
      {open ? (
        <span role="tooltip" style={{
          position: "absolute", ...pos, zIndex: 50, whiteSpace: "nowrap",
          background: "var(--surface-inverse)", color: "var(--text-inverse)",
          font: "var(--type-body-s)", padding: "7px 12px", borderRadius: "var(--radius-sm)",
          boxShadow: "var(--shadow-2)", pointerEvents: "none",
        }}>{label}</span>
      ) : null}
    </span>
  );
}
