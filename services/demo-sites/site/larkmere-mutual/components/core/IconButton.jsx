import React from "react";
import { Icon } from "./Icon.jsx";

export function IconButton({ icon, label, size = "md", variant = "ghost", disabled = false, onClick, style, ...rest }) {
  const [state, setState] = React.useState("rest");
  const box = size === "sm" ? 32 : size === "lg" ? 48 : 40;
  const glyph = size === "sm" ? 16 : size === "lg" ? 24 : 20;
  const solid = variant === "solid";
  return (
    <button
      {...rest}
      type="button"
      aria-label={label}
      title={label}
      disabled={disabled}
      onClick={disabled ? undefined : onClick}
      onMouseEnter={() => !disabled && setState("hover")}
      onMouseLeave={() => setState("rest")}
      onMouseDown={() => !disabled && setState("press")}
      onMouseUp={() => !disabled && setState("hover")}
      style={{
        width: box, height: box, display: "inline-flex", alignItems: "center", justifyContent: "center",
        borderRadius: "var(--radius-pill)", cursor: disabled ? "not-allowed" : "pointer",
        border: solid ? "1px solid var(--action-primary-bg)" : "1px solid transparent",
        color: disabled ? "var(--action-disabled-fg)" : solid ? "var(--action-primary-fg)" : "var(--ink-700)",
        background: disabled
          ? "var(--action-disabled-bg)"
          : solid
            ? state === "rest" ? "var(--action-primary-bg)" : state === "hover" ? "var(--action-primary-bg-hover)" : "var(--action-primary-bg-press)"
            : state === "rest" ? "transparent" : state === "hover" ? "var(--sand-100)" : "var(--sand-200)",
        transform: state === "press" ? "translateY(1px)" : "none",
        transition: "var(--transition-control), transform var(--dur-instant) var(--ease-standard)",
        ...style,
      }}
    >
      <Icon name={icon} size={glyph} />
    </button>
  );
}
