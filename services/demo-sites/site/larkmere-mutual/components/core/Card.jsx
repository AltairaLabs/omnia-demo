import React from "react";

export function Card({ children, featured = false, interactive = false, padding = "var(--space-7)", as = "div", style, onClick, ...rest }) {
  const [hover, setHover] = React.useState(false);
  const Tag = as;
  return (
    <Tag
      {...rest}
      onClick={onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        background: "var(--surface-card)",
        border: "1px solid " + (featured ? "var(--mint-300)" : "var(--border-subtle)"),
        borderRadius: "var(--radius-lg)",
        boxShadow: featured ? "var(--shadow-2)" : interactive && hover ? "var(--shadow-2)" : "var(--shadow-1)",
        transform: interactive && hover ? "translateY(-2px)" : "none",
        padding,
        cursor: interactive ? "pointer" : undefined,
        transition: "var(--transition-surface), background-color var(--dur-fast) var(--ease-standard)",
        boxSizing: "border-box",
        ...style,
      }}
    >
      {children}
    </Tag>
  );
}

export function CardDivider({ style }) {
  return <div style={{ height: 1, background: "var(--border-subtle)", margin: "var(--space-5) 0", ...style }} />;
}
