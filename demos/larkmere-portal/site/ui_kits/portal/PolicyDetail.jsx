import React, { useState } from "react";
import { Card, CardDivider } from "../../components/core/Card.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Button } from "../../components/core/Button.jsx";
import { Icon } from "../../components/core/Icon.jsx";
import { Tabs } from "../../components/navigation/Tabs.jsx";
import { Switch } from "../../components/forms/Switch.jsx";
import { SectionHead } from "./Chrome.jsx";

const COVERAGE = [
  ["Dwelling", "$420,000", "Rebuilding cost, indexed each renewal"],
  ["Contents", "$210,000", "Including $12,000 of listed items"],
  ["Loss of use", "$84,000", "Up to 24 months"],
  ["Liability", "$500,000", "Per occurrence"],
  ["Deductible", "$1,000", "Per claim, all perils"],
];

const CLAIMS = [
  ["CLM-2026-004182", "Storm — roof and gutter", "9 Feb 2026", "Settled", "$6,412.00", "positive"],
  ["CLM-2023-001904", "Water — kitchen supply line", "3 Nov 2023", "Settled", "$2,180.00", "positive"],
];

const DOCS = [
  ["Policy schedule 2026", "PDF · 6 pages", "1 Oct 2025"],
  ["Certificate of insurance", "PDF · 1 page", "1 Oct 2025"],
  ["Renewal notice", "PDF · 2 pages", "18 Aug 2026"],
  ["2025 dividend statement", "PDF · 1 page", "15 Mar 2026"],
];

export function PolicyDetail({ onNavigate, onToast }) {
  const [tab, setTab] = useState("coverage");
  const [autopay, setAutopay] = useState(true);
  return (
    <div style={{ display: "grid", gridTemplateColumns: "7fr 4fr", gap: "var(--space-9)", alignItems: "start", maxWidth: 1140 }}>
      <div>
        <div style={{ display: "flex", alignItems: "center", gap: "var(--space-5)", marginBottom: "var(--space-6)" }}>
          <span style={{ font: "var(--type-figure-s)", color: "var(--text-muted)" }}>HO-4471-01</span>
          <Badge tone="positive" dot>Active</Badge>
          <span style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>Renews 1 Oct 2026</span>
        </div>
        <Tabs value={tab} onChange={setTab} items={[
          { value: "coverage", label: "Coverage" },
          { value: "claims", label: "Claims", count: 2 },
          { value: "docs", label: "Documents", icon: "file-text" },
        ]} />
        <div style={{ marginTop: "var(--space-8)" }}>
          {tab === "coverage" ? (
            <>
              <SectionHead action={<span style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>Limit</span>}>What's covered</SectionHead>
              {COVERAGE.map(([name, limit, note]) => (
                <div key={name} style={{ display: "flex", justifyContent: "space-between", gap: "var(--space-7)", padding: "var(--space-5) 0", borderBottom: "1px solid var(--border-subtle)" }}>
                  <div>
                    <div style={{ font: "var(--type-heading-s)" }}>{name}</div>
                    <div style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", marginTop: 2 }}>{note}</div>
                  </div>
                  <div style={{ font: "var(--type-figure)", fontVariantNumeric: "tabular-nums", whiteSpace: "nowrap" }}>{limit}</div>
                </div>
              ))}
            </>
          ) : null}
          {tab === "claims" ? (
            <>
              <SectionHead action={<Button variant="ghost" size="sm" icon="life-buoy" onClick={() => onNavigate("claims")}>File a claim</Button>}>Claim history</SectionHead>
              {CLAIMS.map((c) => (
                <div key={c[0]} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "var(--space-7)", padding: "var(--space-5) 0", borderBottom: "1px solid var(--border-subtle)" }}>
                  <div>
                    <div style={{ font: "var(--type-heading-s)" }}>{c[1]}</div>
                    <div style={{ font: "var(--type-figure-s)", color: "var(--text-muted)", marginTop: 2 }}>{c[0]} · {c[2]}</div>
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: "var(--space-6)" }}>
                    <Badge tone={c[5]}>{c[3]}</Badge>
                    <span style={{ font: "var(--type-figure)", fontVariantNumeric: "tabular-nums" }}>{c[4]}</span>
                  </div>
                </div>
              ))}
            </>
          ) : null}
          {tab === "docs" ? (
            <>
              <SectionHead action={<span style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>Issued</span>}>Documents</SectionHead>
              {DOCS.map((d) => (
                <div key={d[0]} style={{ display: "flex", justifyContent: "space-between", alignItems: "center", gap: "var(--space-7)", padding: "var(--space-5) 0", borderBottom: "1px solid var(--border-subtle)" }}>
                  <div style={{ display: "flex", alignItems: "center", gap: "var(--space-4)" }}>
                    <span style={{ color: "var(--ink-400)", display: "inline-flex" }}><Icon name="file-text" size={20} /></span>
                    <div>
                      <div style={{ font: "var(--type-body)" }}>{d[0]}</div>
                      <div style={{ font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: 2 }}>{d[1]}</div>
                    </div>
                  </div>
                  <div style={{ display: "flex", alignItems: "center", gap: "var(--space-6)" }}>
                    <span style={{ font: "var(--type-figure-s)", color: "var(--text-muted)" }}>{d[2]}</span>
                    <Button variant="ghost" size="sm" icon="download" onClick={() => onToast({ tone: "positive", title: "Download started.", description: d[0] + " · PDF" })}>Download</Button>
                  </div>
                </div>
              ))}
            </>
          ) : null}
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-7)" }}>
        <Card>
          <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-muted)" }}>Premium</div>
          <div style={{ font: "var(--type-figure-l)", fontVariantNumeric: "tabular-nums", marginTop: "var(--space-4)" }}>$148.20</div>
          <div style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", marginTop: 2 }}>a month · next due 1 Oct 2026</div>
          <CardDivider />
          <Switch id="ap" checked={autopay} onChange={() => setAutopay(!autopay)}
            label="Pay automatically" description="From Everyday checking, five days before it's due." />
          <div style={{ display: "flex", gap: "var(--space-4)", marginTop: "var(--space-6)" }}>
            <Button variant="primary" size="sm" onClick={() => onNavigate("transfer")}>Pay now</Button>
            <Button variant="secondary" size="sm">Change frequency</Button>
          </div>
        </Card>
        <Card>
          <div style={{ font: "var(--type-heading-s)" }}>Your adjuster</div>
          <div style={{ display: "flex", gap: "var(--space-5)", alignItems: "center", marginTop: "var(--space-5)" }}>
            <img src="../../assets/photos/adjuster-ruth.jpg" alt="" width="56" height="56"
              style={{ borderRadius: "var(--radius-md)", objectFit: "cover", display: "block" }} />
            <div>
              <div style={{ font: "var(--type-body)" }}>Ruth Alderman</div>
              <div style={{ font: "var(--type-body-s)", color: "var(--text-muted)", marginTop: 2 }}>Vermont field office · 14 years</div>
            </div>
          </div>
          <CardDivider />
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)", font: "var(--type-body-s)", color: "var(--text-secondary)" }}>
            <Icon name="phone" size={16} />1-800-000-000, extension 2214
          </div>
        </Card>
        <Card>
          <div style={{ font: "var(--type-heading-s)" }}>Something changed at home?</div>
          <p style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", margin: "var(--space-4) 0 var(--space-6)" }}>
            A new roof, a finished basement or a home business can change what you need. Tell us and we'll re-survey at no cost.
          </p>
          <Button variant="secondary" size="sm" fullWidth>Report a change</Button>
        </Card>
      </div>
    </div>
  );
}
