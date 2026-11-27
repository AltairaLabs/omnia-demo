import React from "react";
import { Icon } from "../../components/core/Icon.jsx";
import { Card } from "../../components/core/Card.jsx";
import { Button } from "../../components/core/Button.jsx";
import { Section, Eyebrow, PAGE } from "./Chrome.jsx";

const YEARS = [
  ["1908", "Forty households in Larkmere, Vermont pool their money to insure each other's barns."],
  ["1946", "The mutual takes deposits for the first time, funding mortgages for returning veterans."],
  ["1983", "Retirement accounts open to members. The dividend is paid for the 75th consecutive year."],
  ["2026", "1.2 million members, four states, still no shareholders."],
];

export function About({ onNavigate }) {
  return (
    <main>
      <div style={{ ...PAGE, paddingTop: "var(--space-11)", paddingBottom: "var(--space-9)", maxWidth: "var(--container-text)" }}>
        <Eyebrow gold>About the mutual</Eyebrow>
        <h1 style={{ font: "var(--type-display-l)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-5) 0 var(--space-6)" }}>
          Owned by the people it insures
        </h1>
        <p style={{ font: "var(--type-body-l)", color: "var(--text-secondary)", margin: 0 }}>
          A mutual has no stock and no outside investors. Every policyholder and account holder
          is an owner, which changes what the company optimises for: paying claims properly and
          holding enough surplus to keep doing it.
        </p>
      </div>

      <img src="../../assets/photos/valley-band.jpg" alt=""
        style={{ width: "100%", height: 380, objectFit: "cover", display: "block" }} />

      <Section>
        <div style={{ display: "grid", gridTemplateColumns: "5fr 7fr", gap: "var(--space-11)", alignItems: "start" }}>
          <div>
            <Eyebrow>Since 1908</Eyebrow>
            <h2 style={{ font: "var(--type-display-m)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-4) 0 0" }}>
              Four generations of the same argument
            </h2>
          </div>
          <div>
            {YEARS.map(([y, copy]) => (
              <div key={y} style={{ display: "grid", gridTemplateColumns: "96px 1fr", gap: "var(--space-7)", padding: "var(--space-6) 0", borderBottom: "1px solid var(--border-subtle)" }}>
                <div style={{ font: "var(--type-figure)", color: "var(--text-accent)", fontVariantNumeric: "tabular-nums" }}>{y}</div>
                <p style={{ font: "var(--type-body)", color: "var(--text-secondary)", margin: 0 }}>{copy}</p>
              </div>
            ))}
          </div>
        </div>
      </Section>

      <Section tone="sunken">
        <Eyebrow>Our record, in the open</Eyebrow>
        <h2 style={{ font: "var(--type-display-m)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-4) 0 var(--space-9)" }}>
          Claims and surplus, 2025
        </h2>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "var(--space-7)" }}>
          {[
            ["shield", "98.4%", "Of home claims paid", "1,142 claims declined, each with a written reason and an appeal."],
            ["landmark", "$1.9bn", "Held as surplus", "Enough to pay a one-in-two-hundred-year storm season."],
            ["users", "1.2m", "Members", "Across Vermont, New Hampshire, Maine and upstate New York."],
          ].map(([ic, fig, label, note]) => (
            <Card key={label}>
              <span style={{ color: "var(--text-accent)", display: "inline-flex" }}><Icon name={ic} size={24} /></span>
              <div style={{ font: "var(--type-figure-l)", fontVariantNumeric: "tabular-nums", marginTop: "var(--space-5)" }}>{fig}</div>
              <div style={{ font: "var(--type-heading-s)", marginTop: "var(--space-3)" }}>{label}</div>
              <p style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", margin: "var(--space-4) 0 0" }}>{note}</p>
            </Card>
          ))}
        </div>
        <div style={{ marginTop: "var(--space-9)", display: "flex", gap: "var(--space-5)", alignItems: "center", flexWrap: "wrap" }}>
          <Button onClick={() => onNavigate("protection")}>Get a quote</Button>
          <Button variant="secondary" icon="file-text" onClick={() => onNavigate("about")}>Read the 2025 annual report</Button>
          <span style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>PDF · 84 pages</span>
        </div>
      </Section>
    </main>
  );
}
