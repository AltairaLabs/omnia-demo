import React from "react";
import { Icon } from "../core/Icon.jsx";

export function Select({ value, onChange, options = [], placeholder, size = "md", disabled = false, invalid = false, id, style, ...rest }) {
  const [focus, setFocus] = React.useState(false);
  const height = size === "sm" ? "var(--control-height-sm)" : size === "lg" ? "var(--control-height-lg)" : "var(--control-height-md)";
  return (
    <div style={{
      position: "relative", display: "flex", alignItems: "center", height, boxSizing: "border-box",
      background: disabled ? "var(--sand-100)" : "var(--surface-card)",
      border: "1px solid " + (invalid ? "var(--negative-600)" : focus ? "var(--border-focus)" : "var(--border-default)"),
      borderRadius: "var(--radius-md)", boxShadow: focus ? "var(--focus-ring)" : "none",
      transition: "var(--transition-control)", ...style,
    }}>
      <select
        {...rest}
        id={id}
        value={value}
        onChange={onChange}
        disabled={disabled}
        onFocus={() => setFocus(true)}
        onBlur={() => setFocus(false)}
        style={{
          appearance: "none", WebkitAppearance: "none", border: "none", outline: "none", background: "transparent",
          font: "var(--type-body)", color: disabled ? "var(--ink-400)" : "var(--text-primary)",
          padding: "0 38px 0 16px", height: "100%", width: "100%", cursor: disabled ? "not-allowed" : "pointer",
        }}
      >
        {placeholder ? <option value="">{placeholder}</option> : null}
        {options.map((o) => {
          const opt = typeof o === "string" ? { value: o, label: o } : o;
          return <option key={opt.value} value={opt.value}>{opt.label}</option>;
        })}
      </select>
      <span style={{ position: "absolute", right: 10, pointerEvents: "none", color: "var(--ink-500)", display: "inline-flex" }}>
        <Icon name="chevron-down" size={16} />
      </span>
    </div>
  );
}
