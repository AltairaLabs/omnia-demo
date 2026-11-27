import React, { useState } from "react";
import { Button } from "../../components/core/Button.jsx";
import { Icon } from "../../components/core/Icon.jsx";
import { Card, CardDivider } from "../../components/core/Card.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Field } from "../../components/forms/Field.jsx";
import { Input } from "../../components/forms/Input.jsx";
import { Select } from "../../components/forms/Select.jsx";
import { Radio, RadioGroup } from "../../components/forms/Radio.jsx";
import { Tabs } from "../../components/navigation/Tabs.jsx";
import { Section, Eyebrow, PAGE } from "./Chrome.jsx";

const COVER = {
  home: [
    ["Dwelling", "$420,000", "Rebuilding your house at today's costs"],
    ["Contents", "$210,000", "Everything inside, including bikes and tools"],
    ["Loss of use", "$84,000", "Somewhere to live while repairs happen"],
    ["Liability", "$500,000", "If someone is hurt on your property"],
  ],
  auto: [
    ["Liability", "$500,000", "Injury and damage you cause to others"],
    ["Collision", "$1,000 deductible", "Your car, whoever is at fault"],
    ["Comprehensive", "$500 deductible", "Weather, theft, glass"],
    ["Roadside", "Included", "Towing to the nearest Larkmere garage"],
  ],
  life: [
    ["Term", "20 years", "Level premium for the whole term"],
    ["Benefit", "$500,000", "Paid to your beneficiaries, tax-free"],
    ["Conversion", "To age 65", "Switch to permanent cover without a medical"],
    ["Waiver", "Optional", "Premiums paused if you cannot work"],
  ],
};

export function Protection({ onNavigate }) {
  const [tab, setTab] = useState("home");
  const [freq, setFreq] = useState("monthly");
  return (
    <main>
      <div style={{ background: "var(--surface-card)", borderBottom: "1px solid var(--border-subtle)" }}>
        <div style={{ ...PAGE, paddingTop: "var(--space-11)", paddingBottom: "var(--space-11)", display: "grid", gridTemplateColumns: "7fr 5fr", gap: "var(--space-11)", alignItems: "start" }}>
          <div>
            <Eyebrow gold>Protection</Eyebrow>
            <h1 style={{ font: "var(--type-display-l)", letterSpacing: "var(--tracking-tight)", margin: "var(--space-5) 0 var(--space-6)" }}>
              Home and contents cover
            </h1>
            <p style={{ font: "var(--type-body-l)", color: "var(--text-secondary)", maxWidth: "58ch", margin: 0 }}>
              One policy for the building, everything in it, and the liability that comes with
              owning a home. Adjusters are Larkmere employees, not contractors.
            </p>
            <div style={{ display: "flex", gap: "var(--space-7)", marginTop: "var(--space-8)", flexWrap: "wrap" }}>
              {[["shield", "98.4% of claims paid"], ["life-buoy", "Adjuster calls within 1 day"], ["circle-check", "Dividend-eligible"]].map(([ic, l]) => (
                <span key={l} style={{ display: "inline-flex", alignItems: "center", gap: "var(--space-3)", font: "var(--type-body-s)", color: "var(--text-secondary)" }}>
                  <span style={{ color: "var(--text-accent)", display: "inline-flex" }}><Icon name={ic} size={16} /></span>{l}
                </span>
              ))}
            </div>
          </div>
          <Card featured>
            <Eyebrow gold>Estimate your premium</Eyebrow>
            <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)", marginTop: "var(--space-6)" }}>
              <Field label="Home address" htmlFor="addr"><Input id="addr" icon="map-pin" placeholder="Street and city" /></Field>
              <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: "var(--space-5)" }}>
                <Field label="Year built" htmlFor="yr"><Input id="yr" mono placeholder="1978" /></Field>
                <Field label="Cover level" htmlFor="cl"><Select id="cl" options={["Standard", "Extended", "Full replacement"]} /></Field>
              </div>
              <Field label="How you'd like to pay">
                <RadioGroup>
                  <Radio id="pm" name="freq" value="monthly" checked={freq === "monthly"} onChange={() => setFreq("monthly")}
                    label="Monthly" description="$148.20 a month" />
                  <Radio id="pa" name="freq" value="annual" checked={freq === "annual"} onChange={() => setFreq("annual")}
                    label="Once a year" description="$1,712.00 — saves $66.40" />
                </RadioGroup>
              </Field>
              <CardDivider style={{ margin: "var(--space-2) 0" }} />
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
                <span style={{ font: "var(--type-label)", color: "var(--text-secondary)" }}>Estimated premium</span>
                <span style={{ font: "var(--type-figure-l)", fontVariantNumeric: "tabular-nums" }}>
                  {freq === "monthly" ? "$148.20" : "$1,712.00"}
                </span>
              </div>
              <Button fullWidth onClick={() => onNavigate("banking")}>Continue to your quote</Button>
              <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", margin: 0 }}>
                An estimate, not an offer. Your final premium depends on the survey.
              </p>
            </div>
          </Card>
        </div>
      </div>

      <Section>
        <Tabs value={tab} onChange={setTab} items={[
          { value: "home", label: "Home & contents", icon: "house" },
          { value: "auto", label: "Auto", icon: "car" },
          { value: "life", label: "Term life", icon: "heart-handshake" },
        ]} />
        <div style={{ marginTop: "var(--space-8)", display: "grid", gridTemplateColumns: "7fr 5fr", gap: "var(--space-11)", alignItems: "start" }}>
          <div>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", paddingBottom: "var(--space-4)", borderBottom: "1px solid var(--border-subtle)" }}>
              <span style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase" }}>What's covered</span>
              <span style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase" }}>Limit</span>
            </div>
            {COVER[tab].map(([name, limit, note]) => (
              <div key={name} style={{ display: "flex", justifyContent: "space-between", gap: "var(--space-7)", padding: "var(--space-5) 0", borderBottom: "1px solid var(--border-subtle)" }}>
                <div>
                  <div style={{ font: "var(--type-heading-s)" }}>{name}</div>
                  <div style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", marginTop: 2 }}>{note}</div>
                </div>
                <div style={{ font: "var(--type-figure)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>{limit}</div>
              </div>
            ))}
          </div>
          <Card>
            <div style={{ display: "flex", alignItems: "center", gap: "var(--space-4)" }}>
              <span style={{ color: "var(--text-accent)", display: "inline-flex" }}><Icon name="life-buoy" size={24} /></span>
              <h3 style={{ font: "var(--type-heading-m)", margin: 0 }}>Filing a claim</h3>
            </div>
            <ol style={{ margin: "var(--space-6) 0 0", padding: "0 0 0 var(--space-6)", display: "flex", flexDirection: "column", gap: "var(--space-4)", font: "var(--type-body)", color: "var(--text-secondary)" }}>
              <li>Tell us what happened, online or by phone.</li>
              <li>An adjuster calls you within one business day.</li>
              <li>Send photos from the app when it suits you.</li>
              <li>We pay into your Larkmere account, usually within nine days.</li>
            </ol>
            <CardDivider />
            <div style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "var(--space-5)" }}>
              <Badge tone="positive" dot>Adjusters on staff</Badge>
              <Button variant="ghost" icon="chevron-right" iconPosition="right" onClick={() => onNavigate("about")}>Our claims record</Button>
            </div>
          </Card>
        </div>
      </Section>
    </main>
  );
}
