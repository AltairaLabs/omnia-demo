import React, { useState } from "react";
import { Card, CardDivider } from "../../components/core/Card.jsx";
import { Button } from "../../components/core/Button.jsx";
import { Field } from "../../components/forms/Field.jsx";
import { Input } from "../../components/forms/Input.jsx";
import { Select } from "../../components/forms/Select.jsx";
import { Radio, RadioGroup } from "../../components/forms/Radio.jsx";
import { Checkbox } from "../../components/forms/Checkbox.jsx";
import { Dialog } from "../../components/feedback/Dialog.jsx";
import { SectionHead } from "./Chrome.jsx";

const SCHEDULED = [
  ["12 Sep", "Everyday checking → Member savings", "$500.00", "Repeats monthly"],
  ["1 Oct", "Premium — Home & contents", "$148.20", "Automatic"],
  ["1 Oct", "Premium — Auto", "$96.40", "Automatic"],
];

export function Transfer({ onToast }) {
  const [amount, setAmount] = useState("1,200.00");
  const [when, setWhen] = useState("now");
  const [repeat, setRepeat] = useState(false);
  const [confirm, setConfirm] = useState(false);
  return (
    <div style={{ display: "grid", gridTemplateColumns: "6fr 6fr", gap: "var(--space-9)", alignItems: "start", maxWidth: 1040 }}>
      <Card>
        <div style={{ font: "var(--type-heading-m)" }}>Move money</div>
        <p style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", margin: "var(--space-3) 0 var(--space-7)" }}>
          Between your own Larkmere accounts. Transfers made before 4pm arrive the same day.
        </p>
        <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-6)" }}>
          <Field label="From" htmlFor="from" hint="Available today: $4,812.66">
            <Select id="from" options={["Everyday checking •••• 4471", "Member savings •••• 2210"]} />
          </Field>
          <Field label="To" htmlFor="to">
            <Select id="to" options={["Member savings •••• 2210", "24-month certificate •••• 7708", "Everyday checking •••• 4471"]} />
          </Field>
          <Field label="Amount" htmlFor="amt">
            <Input id="amt" mono prefix="$" size="lg" value={amount} onChange={(e) => setAmount(e.target.value)} />
          </Field>
          <Field label="When">
            <RadioGroup>
              <Radio id="now" name="when" value="now" checked={when === "now"} onChange={() => setWhen("now")}
                label="Today" description="Arrives by 6pm if you send it before 4pm" />
              <Radio id="later" name="when" value="later" checked={when === "later"} onChange={() => setWhen("later")}
                label="On a date you choose" description="Up to 12 months ahead" />
            </RadioGroup>
          </Field>
          {when === "later" ? (
            <Field label="Date" htmlFor="dt"><Input id="dt" mono icon="calendar" placeholder="12 / 09 / 2026" /></Field>
          ) : null}
          <Checkbox id="rep" checked={repeat} onChange={() => setRepeat(!repeat)}
            label="Repeat this transfer every month" description="You can stop it at any time." />
          <CardDivider style={{ margin: 0 }} />
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
            <span style={{ font: "var(--type-label)", color: "var(--text-secondary)" }}>Moving</span>
            <span style={{ font: "var(--type-figure-l)", fontVariantNumeric: "tabular-nums" }}>${amount}</span>
          </div>
          <div style={{ display: "flex", gap: "var(--space-4)" }}>
            <Button onClick={() => setConfirm(true)}>Review this transfer</Button>
            <Button variant="ghost">Cancel</Button>
          </div>
        </div>
      </Card>

      <div>
        <SectionHead action={<span style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>Next 30 days</span>}>Scheduled</SectionHead>
        <table style={{ width: "100%", borderCollapse: "collapse" }}>
          <tbody>
            {SCHEDULED.map((r) => (
              <tr key={r[1] + r[0]} style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                <td style={{ padding: "var(--space-5) var(--space-5) var(--space-5) 0", font: "var(--type-figure-s)", color: "var(--text-muted)", whiteSpace: "nowrap" }}>{r[0]}</td>
                <td style={{ padding: "var(--space-5) 0" }}>
                  <div style={{ font: "var(--type-body)" }}>{r[1]}</div>
                  <div style={{ font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: 2 }}>{r[3]}</div>
                </td>
                <td style={{ padding: "var(--space-5) 0 var(--space-5) var(--space-5)", textAlign: "right", font: "var(--type-figure)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>{r[2]}</td>
              </tr>
            ))}
          </tbody>
        </table>
        <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: "var(--space-6)" }}>
          Automatic premium payments run five days before the due date. Stopping one may affect your cover.
        </p>
      </div>

      <Dialog open={confirm} onClose={() => setConfirm(false)} title="Send this transfer?"
        description={"$" + amount + " moves from Everyday checking to Member savings " + (when === "now" ? "today" : "on 12 September") + "."}
        footer={
          <>
            <Button variant="secondary" onClick={() => setConfirm(false)}>Go back</Button>
            <Button onClick={() => { setConfirm(false); onToast({ tone: "positive", title: "Transfer scheduled.", description: "$" + amount + " moves to Member savings " + (when === "now" ? "today by 6pm" : "on 12 September") + "." }); }}>
              Send it
            </Button>
          </>
        } />
    </div>
  );
}
