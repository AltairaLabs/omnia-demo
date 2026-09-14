import React from "react";
import { Icon } from "../core/Icon.jsx";

export function Tabs({ items = [], value, onChange, style }) {
  const [hover, setHover] = React.useState(null);
  return (
    <div role="tablist" style={{ display: "flex", gap: "var(--space-7)", borderBottom: "1px solid var(--border-subtle)", ...style }}>
      {items.map((it) => {
        const active = it.value === value;
        return (
          <button
            key={it.value}
            role="tab"
            aria-selected={active}
            onClick={() => onChange && onChange(it.value)}
            onMouseEnter={() => setHover(it.value)}
            onMouseLeave={() => setHover(null)}
            style={{
              display: "inline-flex", alignItems: "center", gap: "var(--space-3)",
              background: "transparent", border: "none", cursor: "pointer",
              padding: "0 0 10px", marginBottom: -1,
              font: active ? "var(--type-heading-s)" : "var(--type-label)",
              color: active ? "var(--text-primary)" : hover === it.value ? "var(--text-accent)" : "var(--text-secondary)",
              borderBottom: "2px solid " + (active ? "var(--mint-500)" : "transparent"),
              transition: "var(--transition-control)",
            }}
          >
            {it.icon ? <Icon name={it.icon} size={16} /> : null}
            {it.label}
            {it.count != null ? (
              <span style={{ font: "var(--type-figure-s)", color: "var(--text-muted)" }}>{it.count}</span>
            ) : null}
          </button>
        );
      })}
    </div>
  );
}
