import React from "react";
import { Button } from "../../components/core/Button.jsx";
import { Icon } from "../../components/core/Icon.jsx";
import { Card } from "../../components/core/Card.jsx";
import { Section, Eyebrow, PAGE } from "./Chrome.jsx";

const LINES = [
  { id: "protection", icon: "shield", name: "Protection", copy: "Home, auto, umbrella and term life, written by the company you own.", link: "See what's covered" },
  { id: "banking", icon: "landmark", name: "Banking", copy: "Everyday checking with no monthly fee, savings at 4.15% APY.", link: "Compare accounts" },
  { id: "banking", icon: "trending-up", name: "Retirement", copy: "Workplace plans, individual accounts and advice that stays with you.", link: "Plan your retirement" },
];

const FACTS = [
  ["1908", "Founded — and member-owned every year since"],
  ["98.4%", "Of home claims paid last year"],
  ["$412", "Average member dividend for 2025"],
  ["1 day", "To hear from a claim adjuster"],
];

export function Home({ onNavigate }) {
  return (
    <main>
      <div style={{ ...PAGE, paddingTop: "var(--space-12)", paddingBottom: "var(--space-12)" }}>
        <div style={{ display: "grid", gridTemplateColumns: "7fr 5fr", gap: "var(--space-11)", alignItems: "center" }}>
          <div>
            <Eyebrow gold>Member-owned since 1908</Eyebrow>
            <h1 style={{ font: "var(--type-display-xl)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-5) 0 0" }}>
              Money that answers<br />to you.
            </h1>
            <p style={{ font: "var(--type-body-l)", color: "var(--text-secondary)", maxWidth: "56ch", margin: "var(--space-6) 0 0" }}>
              Larkmere has no shareholders. You and 1.2 million other members own it, so the
              surplus comes back to you as a dividend — paid every year for 118 years.
            </p>
            <div style={{ display: "flex", gap: "var(--space-5)", marginTop: "var(--space-8)" }}>
              <Button size="lg" variant="accent" onClick={() => onNavigate("protection")}>Get a quote</Button>
              <Button size="lg" variant="secondary" onClick={() => onNavigate("banking")}>Compare accounts</Button>
            </div>
            <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: "var(--space-6)" }}>
              A quote takes about four minutes. No call required.
            </p>
          </div>
          <img src="../../assets/photos/home-hero.jpg" alt=""
            style={{ width: "100%", height: 460, objectFit: "cover", borderRadius: "var(--radius-2xl)", display: "block" }} />
        </div>
      </div>

      <div style={{ borderTop: "1px solid var(--border-subtle)", borderBottom: "1px solid var(--border-subtle)", background: "var(--surface-card)" }}>
        <div style={{ ...PAGE, display: "grid", gridTemplateColumns: "repeat(4, 1fr)" }}>
          {FACTS.map(([fig, label], i) => (
            <div key={fig} style={{
              padding: "var(--space-8) var(--space-7)",
              borderLeft: i === 0 ? "none" : "1px solid var(--border-subtle)",
            }}>
              <div style={{ font: "var(--type-figure)", fontVariantNumeric: "tabular-nums" }}>{fig}</div>
              <div style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", marginTop: "var(--space-3)" }}>{label}</div>
            </div>
          ))}
        </div>
      </div>

      <Section>
        <Eyebrow>Three lines, one institution</Eyebrow>
        <h2 style={{ font: "var(--type-display-m)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-4) 0 var(--space-9)" }}>
          What you can hold with us
        </h2>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "var(--space-7)" }}>
          {LINES.map((l) => (
            <Card key={l.name} interactive onClick={() => onNavigate(l.id)}>
              <span style={{ color: "var(--text-accent)", display: "inline-flex" }}><Icon name={l.icon} size={24} /></span>
              <h3 style={{ font: "var(--type-heading-m)", margin: "var(--space-5) 0 var(--space-4)" }}>{l.name}</h3>
              <p style={{ font: "var(--type-body)", color: "var(--text-secondary)", margin: 0 }}>{l.copy}</p>
              <div style={{ display: "inline-flex", alignItems: "center", gap: "var(--space-2)", marginTop: "var(--space-6)", font: "var(--type-label)", color: "var(--text-link)" }}>
                {l.link}<Icon name="chevron-right" size={16} />
              </div>
            </Card>
          ))}
        </div>
      </Section>

      <section style={{ background: "var(--surface-inverse)", padding: "var(--space-13) 0" }}>
        <div style={{ ...PAGE, display: "grid", gridTemplateColumns: "5fr 6fr", gap: "var(--space-11)", alignItems: "center" }}>
          <div>
            <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--mint-300)" }}>
              The member dividend
            </div>
            <h2 style={{ font: "var(--type-display-l)", letterSpacing: "var(--tracking-tight)", color: "var(--sand-50)", margin: "var(--space-5) 0 var(--space-6)" }}>
              118 years of payouts
            </h2>
            <p style={{ font: "var(--type-body-l)", color: "var(--green-200)", margin: 0, maxWidth: "48ch" }}>
              When claims run lower than we planned for, the difference is yours. Last year the
              average household received $412, paid into their Larkmere account on 15 March.
            </p>
          </div>
          <div style={{ background: "var(--surface-inverse-soft)", border: "1px solid var(--border-inverse)", borderRadius: "var(--radius-xl)", padding: "var(--space-7)" }}>
            {[["2025", "$412.00"], ["2024", "$386.00"], ["2023", "$401.00"], ["2022", "$355.00"]].map(([y, v]) => (
              <div key={y} style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", padding: "var(--space-5) 0", borderBottom: "1px solid var(--border-inverse)" }}>
                <span style={{ font: "var(--type-body)", color: "var(--green-200)" }}>{y} · average household</span>
                <span style={{ font: "var(--type-figure)", color: "var(--sand-50)", fontVariantNumeric: "tabular-nums" }}>{v}</span>
              </div>
            ))}
          </div>
        </div>
      </section>

      <Section tone="sunken">
        <div style={{ display: "grid", gridTemplateColumns: "4fr 7fr", gap: "var(--space-11)", alignItems: "center" }}>
          <img src="../../assets/photos/member-quote.jpg" alt=""
            style={{ width: "100%", height: 340, objectFit: "cover", borderRadius: "var(--radius-2xl)", display: "block" }} />
          <div>
            <Eyebrow>A member, Vermont</Eyebrow>
            <blockquote style={{
              font: "var(--type-editorial)", color: "var(--text-primary)",
              margin: "var(--space-5) 0 var(--space-6)", maxWidth: "30ch",
            }}>
              "The tree came through the roof on a Sunday. Someone from Larkmere was standing in
              my kitchen on Monday morning."
            </blockquote>
            <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", margin: 0 }}>
              Home claim settled in nine days · member since 2011
            </p>
          </div>
        </div>
      </Section>
    </main>
  );
}
