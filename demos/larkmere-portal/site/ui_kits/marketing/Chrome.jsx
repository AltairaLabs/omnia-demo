import React, { useEffect, useState } from "react";
import { Button } from "../../components/core/Button.jsx";
import { Icon } from "../../components/core/Icon.jsx";

const NAV = [
  { id: "protection", label: "Protection" },
  { id: "banking", label: "Banking" },
  { id: "about", label: "About the mutual" },
];

export const PAGE = { maxWidth: "var(--container-wide)", margin: "0 auto", padding: "0 var(--gutter-page)" };

export function Wordmark({ inverse = false, size = 40 }) {
  return (
    <img
      src={inverse ? "../../assets/logo-lockup-inverse.svg" : "../../assets/logo-lockup.svg"}
      alt="Larkmere Mutual"
      style={{ height: size, display: "block" }}
    />
  );
}

export function SiteHeader({ route, onNavigate }) {
  const [scrolled, setScrolled] = useState(false);
  useEffect(() => {
    const el = document.scrollingElement || document.documentElement;
    const onScroll = () => setScrolled(el.scrollTop > 8);
    window.addEventListener("scroll", onScroll, true);
    return () => window.removeEventListener("scroll", onScroll, true);
  }, []);
  return (
    <header style={{
      position: "sticky", top: 0, zIndex: 40, background: "var(--surface-page)",
      borderBottom: "1px solid " + (scrolled ? "var(--border-subtle)" : "transparent"),
      boxShadow: scrolled ? "var(--shadow-hairline)" : "none",
      transition: "box-shadow var(--dur-base) var(--ease-standard), border-color var(--dur-base) var(--ease-standard)",
    }}>
      <div style={{ ...PAGE, height: 88, display: "flex", alignItems: "center", gap: "var(--space-8)" }}>
        <a href="#" onClick={(e) => { e.preventDefault(); onNavigate("home"); }} style={{ textDecoration: "none" }}>
          <Wordmark />
        </a>
        <nav style={{ display: "flex", gap: "var(--space-7)", flex: 1 }}>
          {NAV.map((n) => (
            <a key={n.id} href="#" onClick={(e) => { e.preventDefault(); onNavigate(n.id); }}
              style={{
                font: route === n.id ? "var(--type-heading-s)" : "var(--type-label)",
                color: route === n.id ? "var(--text-primary)" : "var(--text-secondary)",
                textDecoration: "none", padding: "4px 0", whiteSpace: "nowrap",
                borderBottom: "2px solid " + (route === n.id ? "var(--mint-500)" : "transparent"),
              }}>{n.label}</a>
          ))}
        </nav>
        <a href="#" onClick={(e) => e.preventDefault()} style={{
          font: "var(--type-label)", color: "var(--text-accent)", textDecoration: "none",
          display: "inline-flex", alignItems: "center", gap: "var(--space-2)", whiteSpace: "nowrap",
        }}>
          <Icon name="phone" size={16} />1-800-000-000
        </a>
        {/* DIVERGES FROM THE UPSTREAM EXPORT: upstream sends this to the
            banking page, because in the design project the portal is a separate
            artboard with nowhere to link to. Here both surfaces are served by
            the same binary, so Sign in goes where it says it goes — which is
            also the cut the demo needs: public site, then member area. */}
        <a href="/ui_kits/portal/" style={{ textDecoration: "none" }}>
          <Button variant="secondary" size="sm">Sign in</Button>
        </a>
        <Button variant="accent" size="sm" onClick={() => onNavigate("protection")}>Get a quote</Button>
      </div>
    </header>
  );
}

const FOOT = {
  Protection: ["Home &amp; contents", "Auto", "Umbrella", "Term life"],
  Banking: ["Everyday checking", "Member savings", "Certificates", "Member lending"],
  Retirement: ["Workplace plans", "Individual accounts", "Annuities", "Talk to an advisor"],
  Larkmere: ["About the mutual", "The member dividend", "Claims record", "Careers"],
};

export function SiteFooter({ onNavigate }) {
  return (
    <footer style={{ background: "var(--surface-inverse)", color: "var(--sand-50)", marginTop: "var(--space-13)" }}>
      <div style={{ ...PAGE, padding: "var(--space-12) var(--gutter-page) var(--space-9)" }}>
        <div style={{ display: "grid", gridTemplateColumns: "1.4fr repeat(4, 1fr)", gap: "var(--space-9)" }}>
          <div>
            <Wordmark inverse size={52} />
            <p style={{ font: "var(--type-body-s)", color: "var(--green-200)", marginTop: "var(--space-5)", maxWidth: 260 }}>
              Owned by its members since 1908. No shareholders, no outside investors.
            </p>
          </div>
          {Object.keys(FOOT).map((k) => (
            <div key={k}>
              <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--green-300)" }}>{k}</div>
              <ul style={{ listStyle: "none", margin: "var(--space-5) 0 0", padding: 0, display: "flex", flexDirection: "column", gap: "var(--space-4)" }}>
                {FOOT[k].map((l) => (
                  <li key={l}>
                    <a href="#" onClick={(e) => { e.preventDefault(); onNavigate && onNavigate("about"); }}
                      dangerouslySetInnerHTML={{ __html: l }}
                      style={{ font: "var(--type-body-s)", color: "var(--sand-50)", textDecoration: "none" }} />
                  </li>
                ))}
              </ul>
            </div>
          ))}
        </div>
        <div style={{ borderTop: "1px solid var(--border-inverse)", marginTop: "var(--space-10)", paddingTop: "var(--space-6)", display: "flex", justifyContent: "space-between", gap: "var(--space-7)", flexWrap: "wrap" }}>
          <p style={{ font: "var(--type-body-s)", color: "var(--green-200)", margin: 0, maxWidth: 760 }}>
            Larkmere Mutual Insurance &amp; Banking Company. Deposits insured to the applicable limit. Rates shown are current as of 1 September 2026 and may change. Insurance products are underwritten by Larkmere Mutual; banking products are offered through Larkmere Mutual Bank. A fictional company, used for demonstration only.
          </p>
          <div style={{ display: "flex", gap: "var(--space-6)" }}>
            {["Privacy", "Terms", "Accessibility"].map((l) => (
              <a key={l} href="#" onClick={(e) => e.preventDefault()} style={{ font: "var(--type-body-s)", color: "var(--sand-50)", textDecoration: "none" }}>{l}</a>
            ))}
          </div>
        </div>
      </div>
    </footer>
  );
}

export function Eyebrow({ children, gold = false }) {
  return (
    <div style={{
      font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase",
      color: gold ? "var(--text-accent)" : "var(--text-muted)",
    }}>{children}</div>
  );
}

export function Section({ children, tone = "page", style }) {
  return (
    <section style={{
      background: tone === "sunken" ? "var(--surface-sunken)" : tone === "inverse" ? "var(--surface-inverse)" : "var(--surface-page)",
      padding: "var(--space-13) 0", ...style,
    }}>
      <div style={PAGE}>{children}</div>
    </section>
  );
}
