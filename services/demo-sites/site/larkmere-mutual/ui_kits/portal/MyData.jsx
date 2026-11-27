import React, { useState, useEffect, useCallback } from "react";
import { Card, CardDivider } from "../../components/core/Card.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Button } from "../../components/core/Button.jsx";
import { Switch } from "../../components/forms/Switch.jsx";
import { Dialog } from "../../components/feedback/Dialog.jsx";
import { SectionHead } from "./Chrome.jsx";

// Your data: what the society's assistant may remember about the member, what
// it does remember, and the right to have all of it erased.
//
// Every call goes to the site's /api/me/ routes, which the site forwards to
// Omnia's end-user plane with a token for the signed-in member (server.go,
// myDataProxy). Omnia picks the member from that token alone, so this page
// can only ever read and change Ellen's own data.
//
// Consent here is Omnia's: privacy-api holds the grants, memory-api refuses a
// memory in a category that needs a grant the member has not given, and a
// revocation deletes what is already stored in that category.
const API = "../../api/me/";

// The consent categories, in the order a member reads them. "needsGrant"
// matches Omnia's registry (ee/pkg/privacy/consent.go): these three are
// facts about the person, stored only with the member's say-so.
const CATEGORIES = [
  { id: "memory:health", label: "Health and accessibility", needsGrant: true,
    description: "A disability, a condition, or how you need us to communicate with you." },
  { id: "memory:identity", label: "Identity", needsGrant: true,
    description: "Who speaks for you, name changes, and how you prefer to be addressed." },
  { id: "memory:location", label: "Where you live", needsGrant: true,
    description: "Your address, a move, or a second property." },
  { id: "memory:preferences", label: "Preferences", needsGrant: false,
    description: "How you like things done: a contact time, a channel." },
  { id: "memory:context", label: "Your products", needsGrant: false,
    description: "What you hold with us, and what you have asked about it." },
  { id: "memory:history", label: "Past conversations", needsGrant: false,
    description: "What you told the assistant before, so you need not repeat it." },
];
const LABEL = Object.fromEntries(CATEGORIES.map((c) => [c.id, c.label]));

const STATUS = {
  pending: ["Received", "info"],
  in_progress: ["In progress", "info"],
  completed: ["Completed", "positive"],
  failed: ["Needs attention", "negative"],
};

async function call(method, route, body) {
  const resp = await fetch(API + route, {
    method,
    headers: body ? { "Content-Type": "application/json" } : undefined,
    body: body ? JSON.stringify(body) : undefined,
  });
  const text = await resp.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch (_) { data = null; }
  if (!resp.ok) {
    const msg = (data && (data.error || data.message)) || text || resp.statusText;
    throw new Error(resp.status + " " + msg);
  }
  return data;
}

function when(ts) {
  if (!ts) return "";
  const d = new Date(ts);
  if (isNaN(d)) return "";
  return d.toLocaleString("en-GB", { day: "numeric", month: "short", hour: "numeric", minute: "2-digit" });
}

function categoryOf(m) {
  return (m.metadata && m.metadata.consent_category) || m.consent_category || "";
}

export function MyData({ onToast }) {
  const [consent, setConsent] = useState(null);
  const [memories, setMemories] = useState(null);
  const [requests, setRequests] = useState([]);
  const [error, setError] = useState(null);
  const [busy, setBusy] = useState("");
  const [confirm, setConfirm] = useState(false);

  const loadMemories = useCallback(() =>
    call("GET", "memories?limit=50").then((d) => setMemories((d && d.memories) || [])), []);
  const loadRequests = useCallback(() =>
    call("GET", "privacy/erasure").then((d) => setRequests(Array.isArray(d) ? d : (d && d.requests) || [])), []);

  const load = useCallback(() => {
    setError(null);
    return Promise.all([
      call("GET", "privacy/consent").then(setConsent),
      loadMemories(),
      loadRequests(),
    ]).catch((e) => setError(e.message));
  }, [loadMemories, loadRequests]);

  useEffect(() => { load(); }, [load]);

  // An erasure runs in the background: follow it until it settles, then show
  // what is left (nothing, if it worked).
  const open = requests.some((r) => r.status === "pending" || r.status === "in_progress");
  useEffect(() => {
    if (!open) return undefined;
    const t = setInterval(() => {
      loadRequests().then(loadMemories).catch((e) => setError(e.message));
    }, 2000);
    return () => clearInterval(t);
  }, [open, loadRequests, loadMemories]);

  const granted = new Set((consent && consent.grants) || []);

  async function toggle(cat, on) {
    setBusy(cat.id);
    try {
      const next = await call("PUT", "privacy/consent", on
        ? { grants: [cat.id], revocations: [] }
        : { grants: [], revocations: [cat.id] });
      setConsent(next);
      if (!on) {
        // privacy-api pushes the revocation to memory-api after it commits,
        // and memory-api deletes the memories in that category. Look again a
        // moment later to show them gone.
        setTimeout(() => loadMemories().catch((e) => setError(e.message)), 1500);
      }
      onToast({
        tone: "positive",
        title: on ? cat.label + ": allowed." : cat.label + ": withdrawn.",
        description: on
          ? "The assistant may remember this kind of detail from now on."
          : "The assistant will not remember this kind of detail, and what it had is deleted.",
      });
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy("");
    }
  }

  async function forget(m) {
    setBusy(m.id);
    try {
      await call("DELETE", "memories/" + encodeURIComponent(m.id));
      await loadMemories();
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy("");
    }
  }

  async function erase() {
    setConfirm(false);
    setBusy("erase");
    try {
      await call("POST", "privacy/erasure", { reason: "gdpr_erasure" });
      await loadRequests();
      onToast({ tone: "positive", title: "Erasure requested.", description: "We are deleting your conversations and everything the assistant remembers." });
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy("");
    }
  }

  return (
    <div style={{ display: "grid", gridTemplateColumns: "7fr 5fr", gap: "var(--space-9)", alignItems: "start", maxWidth: 1140 }}>
      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-9)" }}>
        {error ? (
          <Card style={{ borderColor: "var(--negative-700)" }}>
            <div style={{ font: "var(--type-body)", color: "var(--negative-700)" }}>Something went wrong: {error}</div>
            <div style={{ marginTop: "var(--space-4)" }}><Button size="sm" variant="secondary" onClick={load}>Try again</Button></div>
          </Card>
        ) : null}

        <div>
          <SectionHead>What the assistant may remember</SectionHead>
          <Card>
            {CATEGORIES.map((c, i) => (
              <React.Fragment key={c.id}>
                {i === 3 ? <CardDivider /> : null}
                <Switch
                  id={"consent-" + c.id}
                  label={c.label}
                  description={c.needsGrant ? c.description : c.description + " Kept to help you; no permission needed."}
                  checked={c.needsGrant ? granted.has(c.id) : true}
                  disabled={!consent || !c.needsGrant || busy === c.id}
                  onChange={(e) => toggle(c, e.target.checked)}
                  style={{ padding: "var(--space-4) 0" }}
                />
              </React.Fragment>
            ))}
          </Card>
        </div>

        <div>
          <SectionHead action={<Button size="sm" variant="ghost" icon="refresh-cw" onClick={load}>Refresh</Button>}>
            What it remembers about you
          </SectionHead>
          {memories === null ? (
            <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)" }}>Loading…</p>
          ) : memories.length === 0 ? (
            <p style={{ font: "var(--type-body)", color: "var(--text-muted)" }}>Nothing. The assistant holds no memories about you.</p>
          ) : (
            <table style={{ width: "100%", borderCollapse: "collapse" }}>
              <tbody>
                {memories.map((m) => (
                  <tr key={m.id} style={{ borderBottom: "1px solid var(--border-subtle)" }}>
                    <td style={{ padding: "var(--space-5) var(--space-5) var(--space-5) 0" }}>
                      <div style={{ font: "var(--type-body)" }}>{m.content}</div>
                      <div style={{ font: "var(--type-figure-s)", color: "var(--text-muted)", marginTop: 2 }}>{when(m.created_at)}</div>
                    </td>
                    <td style={{ padding: "var(--space-5) 0", textAlign: "right" }}>
                      {categoryOf(m) ? <Badge tone="neutral">{LABEL[categoryOf(m)] || categoryOf(m)}</Badge> : null}
                    </td>
                    <td style={{ padding: "var(--space-5) 0 var(--space-5) var(--space-5)", textAlign: "right" }}>
                      <Button size="sm" variant="ghost" disabled={busy === m.id} onClick={() => forget(m)}>Forget</Button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          )}
        </div>
      </div>

      <div style={{ display: "flex", flexDirection: "column", gap: "var(--space-7)" }}>
        <Card featured>
          <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-accent)" }}>Your right to erasure</div>
          <div style={{ font: "var(--type-heading-m)", marginTop: "var(--space-4)" }}>Erase my data</div>
          <p style={{ font: "var(--type-body)", color: "var(--text-secondary)", margin: "var(--space-4) 0 var(--space-6)" }}>
            Deletes your conversations with the assistant and everything it remembers about you. Your accounts and policies are not affected.
          </p>
          <Button fullWidth variant="danger" disabled={busy === "erase" || open} onClick={() => setConfirm(true)}>
            {open ? "Erasure in progress" : "Erase my data"}
          </Button>
          {requests.length ? <CardDivider /> : null}
          {requests.map((r) => {
            const [label, tone] = STATUS[r.status] || [r.status, "neutral"];
            return (
              <div key={r.id} style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: "var(--space-4)", padding: "var(--space-3) 0" }}>
                <div>
                  <div style={{ font: "var(--type-body-s)" }}>Requested {when(r.createdAt)}</div>
                  {r.status === "completed" ? (
                    <div style={{ font: "var(--type-figure-s)", color: "var(--text-muted)", marginTop: 2 }}>
                      {r.sessionsDeleted || 0} conversation{r.sessionsDeleted === 1 ? "" : "s"} deleted, {when(r.completedAt)}
                    </div>
                  ) : null}
                  {r.errors && r.errors.length ? (
                    <div style={{ font: "var(--type-figure-s)", color: "var(--negative-700)", marginTop: 2 }}>{r.errors.join("; ")}</div>
                  ) : null}
                </div>
                <Badge tone={tone} dot>{label}</Badge>
              </div>
            );
          })}
        </Card>
        <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", margin: 0 }}>
          We keep a record that your request was made and completed, as the law requires. It names you only by a reference we cannot reverse.
        </p>
      </div>

      <Dialog
        open={confirm}
        onClose={() => setConfirm(false)}
        title="Erase your data?"
        description="Your conversations with the assistant and everything it remembers about you will be deleted. This cannot be undone."
        footer={<>
          <Button variant="secondary" onClick={() => setConfirm(false)}>Cancel</Button>
          <Button variant="danger" onClick={erase}>Erase my data</Button>
        </>}
      />
    </div>
  );
}
