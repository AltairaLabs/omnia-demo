import React, { useState } from "react";
import { Button } from "../../components/core/Button.jsx";
import { Card } from "../../components/core/Card.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Tag } from "../../components/core/Tag.jsx";
import { Section, Eyebrow, PAGE } from "./Chrome.jsx";

const ACCOUNTS = [
  { name: "Everyday checking", rate: "0.75%", label: "APY", min: "$0", fee: "None", note: "Fee-free, with a debit card and paper checks if you want them.", featured: false },
  { name: "Member savings", rate: "4.15%", label: "APY", min: "$100", fee: "None", note: "Rate applies to the whole balance. Move money in and out any time.", featured: true },
  { name: "24-month certificate", rate: "4.60%", label: "APY", min: "$1,000", fee: "Early-withdrawal", note: "Locked for two years. Interest paid monthly or at maturity.", featured: false },
];

const RATES = [
  ["Member savings", "4.15%", "$100", "Daily"],
  ["12-month certificate", "4.35%", "$1,000", "Monthly"],
  ["24-month certificate", "4.60%", "$1,000", "Monthly"],
  ["60-month certificate", "4.25%", "$1,000", "Monthly"],
  ["Retirement savings", "4.40%", "$0", "Daily"],
];

export function Banking({ onNavigate }) {
  const [filter, setFilter] = useState("all");
  const rows = filter === "certificates" ? RATES.filter((r) => r[0].includes("certificate")) : RATES;
  return (
    <main>
      <div style={{ ...PAGE, paddingTop: "var(--space-11)", paddingBottom: "var(--space-9)" }}>
        <Eyebrow gold>Banking</Eyebrow>
        <h1 style={{ font: "var(--type-display-l)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-5) 0 var(--space-6)", maxWidth: "22ch" }}>
          Accounts without the monthly fee
        </h1>
        <p style={{ font: "var(--type-body-l)", color: "var(--text-secondary)", maxWidth: "62ch", margin: 0 }}>
          Deposits fund member lending — mortgages and small-business loans in the same towns
          the deposits come from. Not trading desks.
        </p>
      </div>

      <div style={{ ...PAGE, paddingBottom: "var(--space-11)" }}>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "var(--space-7)" }}>
          {ACCOUNTS.map((a) => (
            <Card key={a.name} featured={a.featured}>
              <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "var(--space-4)", minHeight: 24 }}>
                <h3 style={{ font: "var(--type-heading-m)", margin: 0 }}>{a.name}</h3>
                {a.featured ? <Badge tone="accent">Most opened</Badge> : null}
              </div>
              <div style={{ display: "flex", alignItems: "baseline", gap: "var(--space-3)", marginTop: "var(--space-6)" }}>
                <span style={{ font: "var(--type-figure-l)", color: "var(--text-accent)", fontVariantNumeric: "tabular-nums" }}>{a.rate}</span>
                <span style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-muted)" }}>{a.label}</span>
              </div>
              <p style={{ font: "var(--type-body)", color: "var(--text-secondary)", margin: "var(--space-5) 0 var(--space-6)" }}>{a.note}</p>
              <div style={{ borderTop: "1px solid var(--border-subtle)", paddingTop: "var(--space-5)", display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
                {[["Minimum to open", a.min], ["Monthly fee", a.fee]].map(([k, v]) => (
                  <div key={k} style={{ display: "flex", justifyContent: "space-between", font: "var(--type-body-s)" }}>
                    <span style={{ color: "var(--text-muted)" }}>{k}</span>
                    <span style={{ font: "var(--type-figure-s)" }}>{v}</span>
                  </div>
                ))}
              </div>
              <Button variant={a.featured ? "primary" : "secondary"} fullWidth style={{ marginTop: "var(--space-6)" }}>
                Open an account
              </Button>
            </Card>
          ))}
        </div>
      </div>

      <Section tone="sunken">
        <div style={{ display: "flex", alignItems: "flex-end", justifyContent: "space-between", gap: "var(--space-7)", flexWrap: "wrap" }}>
          <div>
            <Eyebrow>Current rates</Eyebrow>
            <h2 style={{ font: "var(--type-display-m)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-4) 0 0" }}>
              Everything we pay, in one table
            </h2>
          </div>
          <div style={{ display: "flex", gap: "var(--space-3)" }}>
            <Tag selected={filter === "all"} onSelect={() => setFilter("all")}>All accounts</Tag>
            <Tag selected={filter === "certificates"} onSelect={() => setFilter("certificates")}>Certificates</Tag>
          </div>
        </div>
        <div style={{ background: "var(--surface-card)", border: "1px solid var(--border-subtle)", borderRadius: "var(--radius-lg)", marginTop: "var(--space-8)", padding: "var(--space-7)" }}>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <thead>
              <tr style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                {["Account", "APY", "Minimum", "Interest paid"].map((h, i) => (
                  <th key={h} style={{
                    textAlign: i === 0 ? "left" : "right", padding: "0 0 var(--space-4)",
                    font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-muted)",
                  }}>{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((r) => (
                <tr key={r[0]} style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                  {r.map((c, i) => (
                    <td key={i} style={{
                      textAlign: i === 0 ? "left" : "right", padding: "var(--space-5) 0",
                      font: i === 0 ? "var(--type-body)" : "var(--type-figure-s)",
                      color: i === 1 ? "var(--figure-positive)" : "var(--text-primary)",
                      fontVariantNumeric: "tabular-nums",
                    }}>{c}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
          <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", margin: "var(--space-6) 0 0" }}>
            APY — annual percentage yield, the rate after compounding. Current as of 1 September 2026 and may change.
            Certificates charge a fee if you withdraw before maturity.
          </p>
        </div>
      </Section>
    </main>
  );
}
