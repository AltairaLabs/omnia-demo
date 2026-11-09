import React, { useState, useEffect, useRef } from "react";
import { AgentAvatar, UserBubble, AgentReceipt, AgentConfirm } from "../../components/Brand.jsx";

// The Wren panel: a live conversation with the `wren` agent, through the site
// server's assistant proxy (demos/demo-sites/server.go), which asserts Amira's
// identity to the agent's websocket facade. There is no credential on this
// page.
//
// Three things here are not just chat:
//
//   RECEIPTS. The facade sends the browser only CLIENT tool calls; the agent's
//   server-side reads (read_bill, read_usage, ...) never cross the socket. So
//   after each turn the panel asks the account system which calls it served
//   this session (api/assistant/receipts) and shows those: what was actually
//   read, not what the model says it read.
//
//   CONSENT. request_consent is a client tool. The facade sends it here as a
//   `tool_call`; the panel acks it at once (the facade auto-denies an
//   unacknowledged call after 5s), renders the "Needs your OK" card, and the
//   click becomes the tool result:
//     Switch me -> POST api/assistant/consent records the consent in the
//                  account system and returns a consent_id; the panel sends
//                  it back as the result. switch_tariff will not run without
//                  that id (ToolPolicy) and the account system rejects any id
//                  it did not issue to this customer for this change.
//     Not now   -> tool_call_nack. The session records the call as denied,
//                  which is the logged decline; no consent exists.
//
//   RELATIVE URLS. Every route is resolved against this page, so the site
//   works under a path prefix (/wrenmoor-energy/...) with no rewrites.

const OPENERS = [
  "Why is October higher? And is there a cheaper tariff for me?",
  "What's using the most power?",
];

function api(path) {
  return new URL("../../api/assistant/" + path, window.location.href);
}

function socketURL() {
  const u = api("ws");
  u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
  return u.href;
}

// A receipt line from an account-system record: the tool and what it was
// asked about.
function toReceipt(r) {
  return { tool: r.tool, detail: r.detail, refused: /refused/.test(r.outcome) };
}

export function WrenPanel() {
  const [thread, setThread] = useState([
    { from: "agent", text: "Afternoon, Amira. I can explain a bill, look at your usage or find you a cheaper tariff. I'll show you what I checked, and I won't change anything without your OK." },
  ]);
  const [draft, setDraft] = useState("");
  const [busy, setBusy] = useState(false);
  const [offline, setOffline] = useState(false);
  const sockRef = useRef(null);
  const sessionRef = useRef(null);
  const seqRef = useRef(0);
  const turnRef = useRef({ streamed: "", shown: 0 });
  const scrollRef = useRef(null);

  // Fetch the receipts the account system recorded since the last look, and
  // attach them to message `index` (or the latest agent message).
  const pullReceipts = async () => {
    try {
      const u = api("receipts");
      if (sessionRef.current) u.searchParams.set("session", sessionRef.current);
      u.searchParams.set("after", String(seqRef.current));
      const res = await fetch(u, { headers: { Accept: "application/json" } });
      if (!res.ok) return [];
      const body = await res.json();
      seqRef.current = body.last_seq || seqRef.current;
      return (body.receipts || []).map(toReceipt);
    } catch (e) {
      return [];
    }
  };

  const say = async (text) => {
    const receipts = await pullReceipts();
    if (!text && receipts.length === 0) return;
    setThread((t) => [...t, { from: "agent", text, receipts }]);
  };

  // Flush text the agent streamed before a client tool call, so the card
  // appears after the words that lead up to it.
  const flush = async () => {
    const turn = turnRef.current;
    const text = turn.streamed.slice(turn.shown).trim();
    turn.shown = turn.streamed.length;
    await say(text);
  };

  useEffect(() => {
    // Start the receipt cursor at "now", so an earlier visit's calls are not
    // shown as this conversation's.
    pullReceipts();

    const sock = new WebSocket(socketURL());
    sockRef.current = sock;
    const send = (m) => {
      if (sessionRef.current) m.session_id = sessionRef.current;
      if (sock.readyState === WebSocket.OPEN) sock.send(JSON.stringify(m));
    };

    sock.onopen = () => setOffline(false);
    sock.onmessage = async (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      switch (msg.type) {
        case "connected":
          sessionRef.current = msg.session_id || null;
          return;
        case "chunk":
          turnRef.current.streamed += msg.content || "";
          return;
        case "tool_call": {
          const call = msg.tool_call || {};
          // Ack first, always: an unacknowledged call is auto-denied in 5s.
          send({ type: "tool_call_ack", tool_call_ack: { call_id: call.id } });
          if (call.name !== "request_consent") {
            send({ type: "tool_call_nack", tool_call_nack: { call_id: call.id, reason: "This page cannot run " + call.name + "." } });
            return;
          }
          await flush();
          const a = call.arguments || {};
          setThread((t) => [...t, {
            from: "confirm", id: call.id, args: a,
            text: [a.summary || call.consent_message, a.how_to_reverse].filter(Boolean).join(" "),
          }]);
          return;
        }
        case "tool_result":
          // The facade's notice that a client call ended without us (timeout).
          if (msg.tool_result && msg.tool_result.error) {
            setThread((t) => t.map((m) => (m.from === "confirm" && m.id === msg.tool_result.id && !m.state ? { ...m, state: "This request timed out. Ask Wren again when you're ready." } : m)));
          }
          return;
        case "done": {
          const turn = turnRef.current;
          const full = msg.content || turn.streamed;
          const already = turn.streamed.slice(0, turn.shown);
          // `done` may carry the whole turn (including text already shown
          // before a consent card) or only what came after it.
          let rest = full;
          if (turn.shown && full.startsWith(already)) rest = full.slice(turn.shown);
          else if (turn.shown && already.trim() === full.trim()) rest = "";
          turnRef.current = { streamed: "", shown: 0 };
          await say(rest.trim());
          setBusy(false);
          return;
        }
        case "error":
          turnRef.current = { streamed: "", shown: 0 };
          setBusy(false);
          setThread((t) => [...t, { from: "agent", text: "Sorry, I couldn't reach your account just then. Try again in a moment, or call us: a person is always one tap away." }]);
          return;
        default:
      }
    };
    sock.onerror = () => { setOffline(true); setBusy(false); };
    sock.onclose = () => { sockRef.current = null; setOffline(true); setBusy(false); };
    return () => sock.close();
  }, []);

  useEffect(() => {
    const box = scrollRef.current;
    if (box) box.scrollTop = box.scrollHeight;
  }, [thread, busy]);

  const ask = (q) => {
    const text = q.trim();
    if (!text || busy) return;
    setDraft("");
    setThread((t) => [...t, { from: "me", text }]);
    const sock = sockRef.current;
    if (!sock || sock.readyState !== WebSocket.OPEN) {
      setThread((t) => [...t, { from: "agent", text: "I'm not connected to your account right now. Reload the page to try again." }]);
      return;
    }
    setBusy(true);
    const m = { type: "message", content: text };
    if (sessionRef.current) m.session_id = sessionRef.current;
    sock.send(JSON.stringify(m));
  };

  const answer = async (card, yes) => {
    const sock = sockRef.current;
    const mark = (patch) => setThread((t) => t.map((m) => (m === card || (m.from === "confirm" && m.id === card.id) ? { ...m, ...patch } : m)));
    const send = (m) => {
      if (sessionRef.current) m.session_id = sessionRef.current;
      if (sock && sock.readyState === WebSocket.OPEN) sock.send(JSON.stringify(m));
    };
    if (!yes) {
      mark({ state: "no" });
      send({ type: "tool_call_nack", tool_call_nack: { call_id: card.id, reason: "The customer chose Not now." } });
      return;
    }
    mark({ busy: true });
    try {
      const res = await fetch(api("consent"), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ tariff: card.args.tariff, effective_date: card.args.effective_date, session_id: sessionRef.current }),
      });
      if (!res.ok) throw new Error("consent " + res.status);
      const consent = await res.json();
      mark({ busy: false, state: "yes" });
      send({ type: "tool_result", tool_result: { call_id: card.id, result: {
        approved: true, consent_id: consent.consent_id, tariff: consent.tariff, effective_date: consent.effective_date,
      } } });
    } catch (e) {
      mark({ busy: false, state: "We couldn't record your OK just then, so nothing has changed." });
      send({ type: "tool_result", tool_result: { call_id: card.id, error: "The consent could not be recorded; nothing was agreed." } });
    }
  };

  const waiting = thread.some((m) => m.from === "confirm" && !m.state);

  return (
    <aside aria-label="Ask Wren" style={{
      flex: "1 1 340px", minWidth: 0, background: "var(--card)", borderRadius: "var(--r-card-lg)",
      boxShadow: "var(--float), 0 0 0 1px var(--line)", display: "flex", flexDirection: "column", overflow: "hidden",
      position: "sticky", top: 24, maxHeight: "calc(100vh - 48px)",
    }}>
      <div style={{ padding: "18px 22px", borderBottom: "1px solid var(--line-soft)", display: "flex", alignItems: "center", gap: 12 }}>
        <AgentAvatar large />
        <div style={{ display: "flex", flexDirection: "column", gap: 2, minWidth: 0 }}>
          <span style={{ fontSize: 16, fontWeight: 600 }}>Ask Wren</span>
          <span className="muted" style={{ fontSize: 13 }}>{offline ? "Reconnecting to your account…" : "Wren checks first and asks before it changes anything."}</span>
        </div>
      </div>

      <div ref={scrollRef} style={{ padding: 22, display: "flex", flexDirection: "column", gap: 18, overflowY: "auto", minHeight: 220, flex: 1 }}>
        {thread.map((m, i) => {
          if (m.from === "me") return <UserBubble key={i}>{m.text}</UserBubble>;
          if (m.from === "confirm") {
            return (
              <AgentConfirm key={i} text={m.text} busy={m.busy} state={m.state}
                onYes={() => answer(m, true)} onNo={() => answer(m, false)} />
            );
          }
          return (
            <div key={i} style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              {m.text ? <p className="agent-text">{m.text}</p> : null}
              <AgentReceipt items={m.receipts} />
            </div>
          );
        })}
        {busy && !waiting ? (
          <span className="muted" style={{ fontSize: 14, display: "flex", alignItems: "center", gap: 8 }}>
            <AgentAvatar /> Checking your account…
          </span>
        ) : null}
      </div>

      <div style={{ padding: "14px 18px 18px", borderTop: "1px solid var(--line-soft)", display: "flex", flexDirection: "column", gap: 10 }}>
        {thread.length <= 1 ? (
          <div style={{ display: "flex", gap: 8, flexWrap: "wrap" }}>
            {OPENERS.map((o) => <button key={o} className="chip" onClick={() => ask(o)}>{o}</button>)}
          </div>
        ) : null}
        <form onSubmit={(e) => { e.preventDefault(); ask(draft); }}
          style={{ display: "flex", alignItems: "center", gap: 8, padding: "6px 6px 6px 16px", borderRadius: "var(--r-pill)", border: "1px solid var(--input-line)" }}>
          <input value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="Ask about your account…" aria-label="Ask Wren about your account"
            style={{ flex: 1, minWidth: 0, border: 0, outline: "none", background: "transparent", font: "inherit", fontSize: 15, color: "inherit" }} />
          <button type="submit" aria-label="Send" disabled={busy}
            style={{ flex: "none", width: 34, height: 34, borderRadius: "50%", border: 0, background: "var(--moor)", color: "var(--card-ink)", fontSize: 16, display: "flex", alignItems: "center", justifyContent: "center" }}>↑</button>
        </form>
        <span style={{ fontSize: 12, color: "var(--peat-faint)", textAlign: "center" }}>Wren is an AI assistant. A person is always one tap away.</span>
      </div>
    </aside>
  );
}
