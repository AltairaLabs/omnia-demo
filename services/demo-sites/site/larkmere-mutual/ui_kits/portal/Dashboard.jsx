import React from "react";
import { Card, CardDivider } from "../../components/core/Card.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Button } from "../../components/core/Button.jsx";
import { Icon } from "../../components/core/Icon.jsx";
import { SectionHead } from "./Chrome.jsx";

const ACCOUNTS = [
  { name: "Everyday checking", num: "•••• 4471", balance: "$4,812.66", note: "Last deposit 2 Sep · $1,240.00" },
  { name: "Member savings", num: "•••• 2210", balance: "$26,104.19", note: "4.15% APY · interest paid daily" },
  { name: "24-month certificate", num: "•••• 7708", balance: "$10,000.00", note: "Matures 14 Mar 2027" },
];

const POLICIES = [
  { name: "Home & contents", id: "HO-4471-01", premium: "$148.20 / mo", status: ["positive", "Active"], next: "Due 1 Oct" },
  { name: "Auto — 2019 Forester", id: "AU-4471-03", premium: "$96.40 / mo", status: ["positive", "Active"], next: "Due 1 Oct" },
  { name: "Term life — 20 year", id: "TL-4471-02", premium: "$41.00 / mo", status: ["caution", "Payment due"], next: "Due 12 Sep" },
];

const LEDGER = [
  ["6 Sep", "Premium — Home & contents", "HO-4471-01", "−$148.20"],
  ["4 Sep", "Interest — Member savings", "SV-2210", "+$88.14"],
  ["2 Sep", "Deposit — payroll", "DEP-99412", "+$1,240.00"],
  ["1 Sep", "Premium — Auto", "AU-4471-03", "−$96.40"],
  ["29 Aug", "Transfer to Member savings", "TRF-88213", "−$500.00"],
];

export function Dashboard({ onNavigate }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-10)", maxWidth: 1140 }}>
      <Card featured style={{ display: "flex", alignItems: "center", justifyContent: "space-between", gap: "var(--space-9)", flexWrap: "wrap" }}>
        <div>
          <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-accent)" }}>
            Your 2025 member dividend
          </div>
          <div style={{ display: "flex", alignItems: "baseline", gap: "var(--space-5)", marginTop: "var(--space-4)" }}>
            <span style={{ font: "var(--type-figure-l)", fontVariantNumeric: "tabular-nums" }}>$412.00</span>
            <span style={{ font: "var(--type-body)", color: "var(--text-secondary)" }}>paid into Everyday checking on 15 March</span>
          </div>
        </div>
        <Button variant="secondary" icon="file-text">See how it was calculated</Button>
      </Card>

      <div>
        <SectionHead action={<Button variant="ghost" size="sm" icon="arrow-left-right" onClick={() => onNavigate("transfer")}>Move money</Button>}>
          Accounts
        </SectionHead>
        <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: "var(--space-7)" }}>
          {ACCOUNTS.map((a) => (
            <Card key={a.name} interactive onClick={() => onNavigate("transfer")}>
              <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline" }}>
                <span style={{ font: "var(--type-heading-s)" }}>{a.name}</span>
                <span style={{ font: "var(--type-figure-s)", color: "var(--text-muted)" }}>{a.num}</span>
              </div>
              <div style={{ font: "var(--type-figure-l)", fontVariantNumeric: "tabular-nums", marginTop: "var(--space-5)" }}>{a.balance}</div>
              <CardDivider />
              <div style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>{a.note}</div>
            </Card>
          ))}
        </div>
      </div>

      <div style={{ display: "grid", gridTemplateColumns: "7fr 5fr", gap: "var(--space-9)", alignItems: "start" }}>
        <div>
          <SectionHead action={<span style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>Last 30 days</span>}>
            Recent activity
          </SectionHead>
          <table style={{ width: "100%", borderCollapse: "collapse" }}>
            <tbody>
              {LEDGER.map((r) => (
                <tr key={r[2]} style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                  <td style={{ padding: "var(--space-5) var(--space-5) var(--space-5) 0", font: "var(--type-figure-s)", color: "var(--text-muted)", whiteSpace: "nowrap" }}>{r[0]}</td>
                  <td style={{ padding: "var(--space-5) 0", font: "var(--type-body)" }}>{r[1]}</td>
                  <td style={{ padding: "var(--space-5) 0", font: "var(--type-figure-s)", color: "var(--text-muted)" }}>{r[2]}</td>
                  <td style={{
                    padding: "var(--space-5) 0 var(--space-5) var(--space-5)", textAlign: "right", whiteSpace: "nowrap",
                    font: "var(--type-figure)", fontVariantNumeric: "tabular-nums",
                    color: r[3].startsWith("+") ? "var(--figure-positive)" : "var(--figure-neutral)",
                  }}>{r[3]}</td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div>
          <SectionHead action={<Button variant="ghost" size="sm" onClick={() => onNavigate("policy")}>All policies</Button>}>
            Policies
          </SectionHead>
          <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
            {POLICIES.map((p) => (
              <Card key={p.id} interactive onClick={() => onNavigate("policy")} padding="var(--space-6)">
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "var(--space-5)" }}>
                  <div>
                    <div style={{ font: "var(--type-heading-s)" }}>{p.name}</div>
                    <div style={{ font: "var(--type-figure-s)", color: "var(--text-muted)", marginTop: 2 }}>{p.id}</div>
                  </div>
                  <Badge tone={p.status[0]} dot>{p.status[1]}</Badge>
                </div>
                <CardDivider style={{ margin: "var(--space-5) 0" }} />
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", font: "var(--type-body-s)", color: "var(--text-secondary)" }}>
                  <span style={{ font: "var(--type-figure-s)" }}>{p.premium}</span>
                  <span style={{ display: "inline-flex", alignItems: "center", gap: "var(--space-2)" }}>{p.next}<Icon name="chevron-right" size={16} /></span>
                </div>
              </Card>
            ))}
          </div>
        </div>
      </div>
    </div>
  );
}
