import React from "react";
import { Icon } from "../../components/core/Icon.jsx";
import { IconButton } from "../../components/core/IconButton.jsx";
import { Toast } from "../../components/feedback/Toast.jsx";

const NAV = [
  { id: "dashboard", label: "Accounts", icon: "landmark" },
  { id: "policy", label: "Policies", icon: "shield" },
  { id: "claims", label: "Claims", icon: "life-buoy" },
  { id: "transfer", label: "Move money", icon: "arrow-left-right" },
];
// Own ids, not three copies of "dashboard". Pointing them all at the dashboard
// meant a click did nothing at all — the worst behaviour to catch on camera,
// because it reads as a broken page rather than a section with nothing in it.
// App renders a titled empty state for every route it has no screen for.
const SECONDARY = [
  { id: "documents", label: "Documents", icon: "file-text" },
  { id: "retirement", label: "Retirement", icon: "trending-up" },
  { id: "settings", label: "Settings", icon: "settings" },
];

function NavLink({ item, active, onNavigate }) {
  const [hover, setHover] = React.useState(false);
  return (
    <a href="#" onClick={(e) => { e.preventDefault(); onNavigate(item.id); }}
      onMouseEnter={() => setHover(true)} onMouseLeave={() => setHover(false)}
      style={{
        display: "flex", alignItems: "center", gap: "var(--space-4)",
        padding: "11px 14px", borderRadius: "var(--radius-md)", textDecoration: "none",
        font: active ? "var(--type-heading-s)" : "var(--type-label)",
        color: active ? "var(--green-950)" : hover ? "var(--sand-50)" : "var(--green-200)",
        background: active ? "var(--mint-500)" : hover ? "var(--surface-inverse-soft)" : "transparent",
        transition: "var(--transition-control)",
      }}>
      <Icon name={item.icon} size={20} />{item.label}
    </a>
  );
}

export function Rail({ route, onNavigate }) {
  return (
    <aside style={{
      width: 252, flex: "0 0 252px", background: "var(--surface-inverse)",
      padding: "var(--space-7) var(--space-5)", boxSizing: "border-box",
      position: "sticky", top: 0, alignSelf: "flex-start", height: "100vh",
      display: "flex", flexDirection: "column", gap: "var(--space-8)",
    }}>
      <div style={{ padding: "0 8px" }}>
        <img src="../../assets/logo-lockup-inverse.svg" alt="Larkmere Mutual" style={{ height: 44, display: "block" }} />
      </div>
      <nav style={{ display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
        {NAV.map((n) => <NavLink key={n.label} item={n} active={route === n.id} onNavigate={onNavigate} />)}
      </nav>
      <div style={{ borderTop: "1px solid var(--border-inverse)", paddingTop: "var(--space-6)", display: "flex", flexDirection: "column", gap: "var(--space-1)" }}>
        {SECONDARY.map((n) => <NavLink key={n.label} item={n} active={route === n.id} onNavigate={onNavigate} />)}
      </div>
      <div style={{ marginTop: "auto", borderTop: "1px solid var(--border-inverse)", paddingTop: "var(--space-6)", padding: "var(--space-6) 12px 0" }}>
        <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--green-300)" }}>Member since 2011</div>
        <div style={{ font: "var(--type-label)", color: "var(--sand-50)", marginTop: "var(--space-3)" }}>Ellen Whitcomb</div>
        <div style={{ font: "var(--type-figure-s)", color: "var(--green-200)", marginTop: 2 }}>No. 4471-889</div>
      </div>
    </aside>
  );
}

export function TopBar({ title, toast, onDismissToast }) {
  return (
    <>
      <header style={{
        position: "sticky", top: 0, zIndex: 30, background: "var(--surface-page)",
        borderBottom: "1px solid var(--border-subtle)", boxShadow: "var(--shadow-hairline)",
        padding: "0 var(--space-9)", height: 72, display: "flex", alignItems: "center", gap: "var(--space-7)",
      }}>
        <h1 style={{ font: "var(--type-heading-l)", letterSpacing: "var(--tracking-tight)", flex: 1 }}>{title}</h1>
        <span style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>Last signed in 6 Sep, 8:14am</span>
        <IconButton icon="search" label="Search" />
        <IconButton icon="bell" label="Alerts" />
        <IconButton icon="user-round" label="Your profile" />
      </header>
      {toast ? (
        <div style={{ position: "fixed", right: "var(--space-9)", bottom: "var(--space-9)", zIndex: 70 }}>
          <Toast tone={toast.tone} title={toast.title} description={toast.description} onDismiss={onDismissToast} />
        </div>
      ) : null}
    </>
  );
}

export function SectionHead({ children, action }) {
  return (
    <div style={{ display: "flex", alignItems: "baseline", justifyContent: "space-between", gap: "var(--space-7)", paddingBottom: "var(--space-4)", borderBottom: "1px solid var(--border-subtle)", marginBottom: "var(--space-6)" }}>
      <span style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase" }}>{children}</span>
      {action}
    </div>
  );
}
