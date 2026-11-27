import React, { useState } from "react";
import { Card, CardDivider } from "../../components/core/Card.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Button } from "../../components/core/Button.jsx";
import { Tag } from "../../components/core/Tag.jsx";
import { Icon } from "../../components/core/Icon.jsx";
import { Field } from "../../components/forms/Field.jsx";
import { Select } from "../../components/forms/Select.jsx";
import { Input } from "../../components/forms/Input.jsx";
import { Toast } from "../../components/feedback/Toast.jsx";
import { SectionHead } from "./Chrome.jsx";

const OPEN = {
  id: "CLM-2026-004182",
  what: "Storm — roof and gutter",
  policy: "HO-4471-01 · Home & contents",
  steps: [
    ["Reported", "9 Feb, 4:12pm", true],
    ["Adjuster assigned — Ruth Alderman", "10 Feb, 9:02am", true],
    ["Photos received", "11 Feb, 7:45pm", true],
    ["Estimate approved", "16 Feb, 11:20am", true],
    ["Payment sent to Everyday checking", "18 Feb", false],
  ],
};

const PAST = [
  ["CLM-2023-001904", "Water — kitchen supply line", "3 Nov 2023", "$2,180.00", "Settled"],
  ["CLM-2021-000771", "Auto — windscreen", "22 Jun 2021", "$480.00", "Settled"],
  ["CLM-2019-000318", "Theft — bicycle", "8 Aug 2019", "$0.00", "Declined"],
];

export function Claims({ onToast }) {
  const [filter, setFilter] = useState("all");
  const [what, setWhat] = useState("");
  const rows = filter === "declined" ? PAST.filter((r) => r[4] === "Declined") : PAST;
  return (
    <div style={{ display: "grid", gridTemplateColumns: "7fr 5fr", gap: "var(--space-9)", alignItems: "start", maxWidth: 1140 }}>
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-9)" }}>
        <div>
          <SectionHead action={<Badge tone="info" dot>In progress</Badge>}>Your open claim</SectionHead>
          <Card>
            <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: "var(--space-5)" }}>
              <div>
                <div style={{ font: "var(--type-heading-m)" }}>{OPEN.what}</div>
                <div style={{ font: "var(--type-figure-s)", color: "var(--text-muted)", marginTop: "var(--space-3)" }}>{OPEN.id} · {OPEN.policy}</div>
              </div>
              <div style={{ textAlign: "right" }}>
                <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-muted)" }}>Approved</div>
                <div style={{ font: "var(--type-figure)", fontVariantNumeric: "tabular-nums", marginTop: 2 }}>$6,412.00</div>
              </div>
            </div>
            <CardDivider />
            <div>
              {OPEN.steps.map(([label, when, done], i) => (
                <div key={label} style={{ display: "grid", gridTemplateColumns: "24px 1fr auto", gap: "var(--space-4)", alignItems: "start", padding: "var(--space-4) 0" }}>
                  <span style={{ color: done ? "var(--positive-700)" : "var(--ink-300)", display: "inline-flex", marginTop: 1 }}>
                    <Icon name={done ? "circle-check" : "circle-dashed"} size={20} />
                  </span>
                  <span style={{ font: done ? "var(--type-body)" : "var(--type-body)", color: done ? "var(--text-primary)" : "var(--text-muted)" }}>{label}</span>
                  <span style={{ font: "var(--type-figure-s)", color: "var(--text-muted)", whiteSpace: "nowrap" }}>{when}</span>
                </div>
              ))}
            </div>
            <CardDivider />
            <div style={{ display: "flex", gap: "var(--space-4)" }}>
              <Button size="sm" icon="camera" onClick={() => onToast({ tone: "positive", title: "Photo upload opened.", description: "Send up to 20 photos from your phone or this browser." })}>Add photos</Button>
              <Button size="sm" variant="secondary" icon="phone">Call Ruth</Button>
            </div>
          </Card>
        </div>

        <div>
          <SectionHead action={
            <div style={{ display: "flex", gap: "var(--space-3)" }}>
              <Tag selected={filter === "all"} onSelect={() => setFilter("all")}>All claims</Tag>
              <Tag selected={filter === "declined"} onSelect={() => setFilter("declined")}>Declined</Tag>
            </div>
          }>Claim history</SectionHead>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <tbody>
              {rows.map((r) => (
                <tr key={r[0]} style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                  <td style={{ padding: "var(--space-5) var(--space-5) var(--space-5) 0" }}>
                    <div style={{ font: "var(--type-body)" }}>{r[1]}</div>
                    <div style={{ font: "var(--type-figure-s)", color: "var(--text-muted)", marginTop: 2 }}>{r[0]}</div>
                  </td>
                  <td style={{ padding: "var(--space-5) 0", font: "var(--type-figure-s)", color: "var(--text-muted)", whiteSpace: "nowrap" }}>{r[2]}</td>
                  <td style={{ padding: "var(--space-5) 0", textAlign: "right" }}>
                    <Badge tone={r[4] === "Declined" ? "neutral" : "positive"}>{r[4]}</Badge>
                  </td>
                  <td style={{ padding: "var(--space-5) 0 var(--space-5) var(--space-5)", textAlign: "right", font: "var(--type-figure)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>{r[3]}</td>
                </tr>
              ))}
            </tbody>
          </table>
          {filter === "declined" ? (
            <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: "var(--space-5)" }}>
              Every declined claim comes with a written reason and an appeal you can start here.
            </p>
          ) : null}
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-7)" }}>
        <Card featured>
          <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-accent)" }}>File a claim</div>
          <div style={{ font: "var(--type-heading-m)", marginTop: "var(--space-4)" }}>Tell us what happened</div>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)", marginTop: "var(--space-6)" }}>
            <Field label="Which policy" htmlFor="pol">
              <Select id="pol" options={["Home & contents — HO-4471-01", "Auto — AU-4471-03", "Term life — TL-4471-02"]} />
            </Field>
            <Field label="What happened" htmlFor="wh" hint="A sentence is enough. An adjuster will call you within one business day.">
              <Input id="wh" value={what} onChange={(e) => setWhat(e.target.value)} placeholder="A tree came through the roof" />
            </Field>
            <Field label="When" htmlFor="wn"><Input id="wn" mono placeholder="06 / 09 / 2026" icon="calendar" /></Field>
            <Button fullWidth onClick={() => onToast({ tone: "positive", title: "Claim started.", description: "Reference CLM-2026-004219. Ruth Alderman calls you by tomorrow, 5pm." })}>
              Start this claim
            </Button>
          </div>
        </Card>
        <Toast tone="info" title="Storm season is open." description="Photograph your roof and gutters now — it makes any winter claim faster to settle." />
      </div>
    </div>
  );
}
