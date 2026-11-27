import React from "react";
import { Icon } from "../core/Icon.jsx";

export function Input({
  value, onChange, placeholder, type = "text", size = "md", icon, prefix, suffix,
  mono = false, invalid = false, disabled = false, id, style, ...rest
}) {
  const [focus, setFocus] = React.useState(false);
  const [hover, setHover] = React.useState(false);
  const height = size === "sm" ? "var(--control-height-sm)" : size === "lg" ? "var(--control-height-lg)" : "var(--control-height-md)";
  return (
    <div
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: "flex", alignItems: "center", gap: "var(--space-3)",
        height, padding: "0 16px", boxSizing: "border-box",
        background: disabled ? "var(--sand-100)" : "var(--surface-card)",
        border: "1px solid " + (invalid ? "var(--negative-600)" : focus ? "var(--border-focus)" : hover && !disabled ? "var(--border-strong)" : "var(--border-default)"),
        borderRadius: "var(--radius-md)",
        boxShadow: focus ? "var(--focus-ring)" : "none",
        transition: "var(--transition-control)",
        ...style,
      }}
    >
      {icon ? <span style={{ color: "var(--ink-400)", display: "inline-flex" }}><Icon name={icon} size={16} /></span> : null}
      {prefix ? <span style={{ font: "var(--type-figure-s)", color: "var(--text-muted)" }}>{prefix}</span> : null}
      <input
        {...rest}
        id={id}
        type={type}
        value={value}
        onChange={onChange}
        placeholder={placeholder}
        disabled={disabled}
        onFocus={() => setFocus(true)}
        onBlur={() => setFocus(false)}
        style={{
          flex: 1, minWidth: 0, border: "none", outline: "none", background: "transparent",
          font: mono ? "var(--type-figure-s)" : "var(--type-body)",
          fontVariantNumeric: mono ? "tabular-nums" : undefined,
          color: disabled ? "var(--ink-400)" : "var(--text-primary)",
        }}
      />
      {suffix ? <span style={{ font: "var(--type-figure-s)", color: "var(--text-muted)" }}>{suffix}</span> : null}
    </div>
  );
}
