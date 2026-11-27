import React from "react";
import { Icon } from "./Icon.jsx";

const SIZES = {
  sm: { height: "var(--control-height-sm)", padding: "0 16px", font: "var(--type-label)", icon: 16 },
  md: { height: "var(--control-height-md)", padding: "0 22px", font: "var(--type-label)", icon: 18 },
  lg: { height: "var(--control-height-lg)", padding: "0 30px", font: "600 17px/1.2 var(--font-sans)", icon: 20 },
};

function palette(variant, state) {
  const p = {
    primary: {
      rest: { background: "var(--action-primary-bg)", color: "var(--action-primary-fg)", border: "1px solid var(--action-primary-bg)" },
      hover: { background: "var(--action-primary-bg-hover)", border: "1px solid var(--action-primary-bg-hover)" },
      press: { background: "var(--action-primary-bg-press)", border: "1px solid var(--action-primary-bg-press)" },
    },
    secondary: {
      rest: { background: "var(--action-secondary-bg)", color: "var(--action-secondary-fg)", border: "1px solid var(--action-secondary-border)", boxShadow: "var(--shadow-hairline)" },
      hover: { background: "var(--action-secondary-bg-hover)", border: "1px solid var(--border-subtle)" },
      press: { background: "var(--action-secondary-bg-press)" },
    },
    accent: {
      rest: { background: "var(--action-accent-bg)", color: "var(--action-accent-fg)", border: "1px solid var(--action-accent-bg)", boxShadow: "var(--shadow-mint)" },
      hover: { background: "var(--action-accent-bg-hover)", border: "1px solid var(--action-accent-bg-hover)" },
      press: { background: "var(--mint-600)", border: "1px solid var(--mint-600)" },
    },
    ghost: {
      rest: { background: "transparent", color: "var(--text-accent)", border: "1px solid transparent" },
      hover: { background: "var(--action-ghost-bg-hover)" },
      press: { background: "var(--sand-200)" },
    },
    danger: {
      rest: { background: "var(--action-danger-bg)", color: "var(--white)", border: "1px solid var(--action-danger-bg)" },
      hover: { background: "var(--action-danger-bg-hover)", border: "1px solid var(--action-danger-bg-hover)" },
      press: { background: "var(--negative-700)", border: "1px solid var(--negative-700)" },
    },
  }[variant] || {};
  return { ...p.rest, ...(state === "hover" ? p.hover : null), ...(state === "press" ? p.press : null) };
}

export function Button({
  children, variant = "primary", size = "md", icon, iconPosition = "left",
  fullWidth = false, disabled = false, type = "button", href, onClick, style, ...rest
}) {
  const [state, setState] = React.useState("rest");
  const s = SIZES[size] || SIZES.md;
  const Tag = href ? "a" : "button";
  const glyph = icon ? <Icon name={icon} size={s.icon} /> : null;
  return (
    <Tag
      {...rest}
      href={href}
      type={href ? undefined : type}
      disabled={href ? undefined : disabled}
      onClick={disabled ? undefined : onClick}
      onMouseEnter={() => !disabled && setState("hover")}
      onMouseLeave={() => setState("rest")}
      onMouseDown={() => !disabled && setState("press")}
      onMouseUp={() => !disabled && setState("hover")}
      style={{
        display: "inline-flex", alignItems: "center", justifyContent: "center", gap: "var(--space-3)",
        height: s.height, padding: s.padding, font: s.font, textDecoration: "none",
        borderRadius: "var(--radius-pill)", letterSpacing: "var(--tracking-snug)", cursor: disabled ? "not-allowed" : "pointer",
        width: fullWidth ? "100%" : undefined, boxSizing: "border-box", flex: "0 0 auto",
        transform: state === "press" ? "translateY(1px)" : "none",
        transition: "var(--transition-control), transform var(--dur-instant) var(--ease-standard)",
        ...(disabled
          ? { background: "var(--action-disabled-bg)", color: "var(--action-disabled-fg)", border: "1px solid var(--action-disabled-bg)" }
          : palette(variant, state)),
        ...style,
      }}
    >
      {glyph && iconPosition === "left" ? glyph : null}
      <span style={{ whiteSpace: "nowrap" }}>{children}</span>
      {glyph && iconPosition === "right" ? glyph : null}
    </Tag>
  );
}
