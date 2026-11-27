import React from "react";
import { Icon } from "./Icon.jsx";

export function Tag({ children, selected = false, onSelect, onRemove, style }) {
  const [hover, setHover] = React.useState(false);
  const clickable = !!onSelect;
  return (
    <span
      onClick={onSelect}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        display: "inline-flex", alignItems: "center", gap: "var(--space-2)",
        height: 34, padding: "0 14px", borderRadius: "var(--radius-pill)",
        font: "var(--type-label)", cursor: clickable ? "pointer" : "default",
        background: selected ? "var(--green-900)" : hover && clickable ? "var(--sand-100)" : "var(--surface-card)",
        color: selected ? "var(--sand-50)" : "var(--ink-700)",
        border: "1px solid " + (selected ? "var(--green-900)" : "var(--border-subtle)"),
        transition: "var(--transition-control)",
        ...style,
      }}
    >
      {children}
      {onRemove ? (
        <span onClick={(e) => { e.stopPropagation(); onRemove(); }} style={{ display: "inline-flex", opacity: 0.7, cursor: "pointer" }}>
          <Icon name="x" size={16} />
        </span>
      ) : null}
    </span>
  );
}
