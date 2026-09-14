import React, { useState, useEffect, useRef } from "react";
import { Button } from "../../components/core/Button.jsx";
import { IconButton } from "../../components/core/IconButton.jsx";
import { Icon } from "../../components/core/Icon.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Input } from "../../components/forms/Input.jsx";

// Suggested openers, shown before the member has said anything. None of them
// carries personal data: the member opens by saying what is wrong, and the
// agent asks for identification on its own turn.
//
// The upstream kit's openers (premiums, dividends, deductibles) belonged to an
// insurance assistant with canned answers. This panel now talks to the
// card-support agent, so they would go unanswered.
const OPENERS = [
  { label: "I've lost my card", text: "Hi, I've lost my card." },
  { label: "Is my card still active?", text: "Is my card still active?" },
  { label: "When is my next payment due?", text: "When is my next payment due?" },
];

// There is deliberately NO chip that fills in the member's own details, and
// none that closes the call for them.
//
// Both existed and both were cut: a portal that offers to type your Social
// Security number for you is not a thing any real product does, and a canned
// mid-conversation reply reads as staged the moment it is on camera. Suggested
// OPENERS are a real pattern and stay; canned answers are not.
//
// So the member's verification details and their sign-off get typed. For the
// recording that is the point — the audience watches a person put an SSN into
// a chat box, which is exactly the thing the rest of the demo is about.

function Chip({ label, onClick }) {
  return (
    <button onClick={onClick} style={{
      height: 34, padding: "0 14px", borderRadius: "var(--radius-pill)", cursor: "pointer",
      background: "var(--surface-card)", border: "1px solid var(--border-subtle)",
      font: "var(--type-label)", color: "var(--ink-700)", whiteSpace: "nowrap",
    }}>{label}</button>
  );
}

function Bubble({ from, children }) {
  const mine = from === "member";
  return (
    <div style={{ display: "flex", justifyContent: mine ? "flex-end" : "flex-start" }}>
      <div style={{
        maxWidth: "86%", padding: "12px 16px",
        font: "var(--type-body-s)", lineHeight: 1.55,
        background: mine ? "var(--green-900)" : "var(--surface-card)",
        color: mine ? "var(--sand-50)" : "var(--text-primary)",
        border: mine ? "1px solid var(--green-900)" : "1px solid var(--border-subtle)",
        borderRadius: "var(--radius-lg)",
        borderBottomRightRadius: mine ? "var(--radius-xs)" : "var(--radius-lg)",
        borderBottomLeftRadius: mine ? "var(--radius-lg)" : "var(--radius-xs)",
        boxShadow: mine ? "none" : "var(--shadow-1)",
      }}>{children}</div>
    </div>
  );
}

function ActionCard({ card, onToast }) {
  return (
    <div style={{
      background: "var(--surface-card)",
      border: "1px solid " + (card.accent ? "var(--mint-300)" : "var(--border-subtle)"),
      borderRadius: "var(--radius-lg)", boxShadow: "var(--shadow-1)",
      padding: "var(--space-5)", marginTop: "var(--space-4)",
    }}>
      <div style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-muted)" }}>
        {card.eyebrow}
      </div>
      <div style={{ font: "var(--type-figure)", fontVariantNumeric: "tabular-nums", marginTop: "var(--space-3)" }}>{card.title}</div>
      <div style={{ font: "var(--type-body-s)", color: "var(--text-secondary)", marginTop: 2 }}>{card.note}</div>
      <Button
        size="sm"
        variant={card.accent ? "accent" : "secondary"}
        style={{ marginTop: "var(--space-5)" }}
        onClick={() => onToast && onToast({
          tone: card.accent ? "positive" : "info",
          title: card.accent ? "Transfer scheduled." : "Opening that for you.",
          description: card.accent ? "$200.00 moves to Member savings today by 6pm." : card.eyebrow,
        })}
      >
        {card.action}
      </Button>
    </div>
  );
}

export function AssistantLauncher({ onClick, open }) {
  const [hover, setHover] = useState(false);
  if (open) return null;
  return (
    <button
      onClick={onClick}
      onMouseEnter={() => setHover(true)}
      onMouseLeave={() => setHover(false)}
      style={{
        position: "fixed", right: "var(--space-9)", bottom: "var(--space-9)", zIndex: 60,
        display: "inline-flex", alignItems: "center", gap: "var(--space-4)",
        height: 54, padding: "0 24px 0 20px", borderRadius: "var(--radius-pill)",
        background: hover ? "var(--mint-600)" : "var(--mint-500)", color: "var(--green-950)",
        border: "none", cursor: "pointer", font: "600 16px/1 var(--font-sans)",
        letterSpacing: "var(--tracking-snug)", boxShadow: "var(--shadow-mint)",
        transform: hover ? "translateY(-1px)" : "none",
        transition: "var(--transition-control), transform var(--dur-base) var(--ease-standard)",
      }}
    >
      <Icon name="message-circle" size={22} />Ask Larkmere
    </button>
  );
}

export function AssistantPanel({ open, onClose, onToast, onNavigate }) {
  const [thread, setThread] = useState([
    // A plain greeting. The AGENT asks for identification, on its own second
    // turn, after the member has said what they want — see the turn note in
    // the deterministic demo script. It must not be asked here:
    // a demo where the member volunteers an SSN into a chat box unprompted is
    // demonstrating the wrong thing, and an agent that verified someone who had
    // never been asked would be worse.
    { from: "bot", text: "Morning, Ellen. I look after cards on your Larkmere account — reporting one lost, checking its status, ordering a replacement. What can I do?" },
  ]);
  const [draft, setDraft] = useState("");
  const [thinking, setThinking] = useState(false);
  const [disclosure, setDisclosure] = useState(null);
  const scrollRef = useRef(null);
  const sockRef = useRef(null);
  const sessionRef = useRef(null);

  // Open the socket when the panel opens; close it when the panel closes.
  //
  // The portal proxies this to the agent facade and asserts the member's
  // identity in headers the browser cannot set (see assistantProxy in
  // demos/larkmere-portal/main.go), so there is no credential on this page.
  React.useEffect(() => {
    if (!open) return undefined;

    const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
    const sock = new WebSocket(proto + "//" + window.location.host + "/api/assistant/ws");
    sockRef.current = sock;

    sock.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }

      if (msg.type === "connected") {
        sessionRef.current = msg.session_id || null;
        // EU AI Act Art. 50(1). The pack DECLARES it
        // (requires_ai_disclosure in its RFC 0013 governance block), the
        // facade advertises it on the first frame, and this is the thing that
        // finally shows it to a person — which is what makes session-api's
        // ai_disclosure_delivered column mean something.
        //
        // Read from the wire, deliberately NOT hardcoded: a panel that always
        // showed the notice would prove nothing about the declaration, and
        // would keep showing it for an agent that had dropped it.
        const c = msg.connected || {};
        setDisclosure(c.ai_disclosure === true);
        return;
      }
      if (msg.type === "done") {
        setThinking(false);
        setThread((t) => [...t, { from: "bot", text: msg.content || "" }]);
        return;
      }
      if (msg.type === "error") {
        setThinking(false);
        setThread((t) => [...t, { from: "bot", text: "Sorry — I couldn't reach your accounts just then. Try again in a moment." }]);
      }
      // `chunk` frames are ignored: this panel renders whole turns. Streaming
      // them token by token is nicer to watch and would mean reassembling
      // partial text, which is not what the demo is about.
    };

    sock.onerror = () => setThinking(false);
    sock.onclose = () => { sockRef.current = null; setThinking(false); };

    return () => { sock.close(); sockRef.current = null; };
  }, [open]);

  React.useLayoutEffect(() => {
    const box = scrollRef.current;
    if (!box) return;
    const pin = () => { box.scrollTop = box.scrollHeight; };
    pin();
    const t0 = setTimeout(pin, 0);
    const t1 = setTimeout(pin, 60);
    const t2 = setTimeout(() => window.lucide && window.lucide.createIcons(), 40);
    return () => { clearTimeout(t0); clearTimeout(t1); clearTimeout(t2); };
  }, [thread, thinking, open]);

  const ask = (q) => {
    if (!q.trim()) return;
    setThread((t) => [...t, { from: "member", text: q }]);
    setDraft("");

    const sock = sockRef.current;
    if (!sock || sock.readyState !== WebSocket.OPEN) {
      setThread((t) => [...t, { from: "bot", text: "I'm not connected to your accounts right now. Close this and open it again." }]);
      return;
    }

    setThinking(true);
    const payload = { type: "message", content: q };
    if (sessionRef.current) payload.session_id = sessionRef.current;
    // Per-turn consent grant. Without it the memory classifier upgrades
    // PII-bearing content to memory:identity and the write is refused at 204 —
    // the case note silently never lands. See PII_CHAT.md.
    payload.consent_grants = ["memory:identity"];
    sock.send(JSON.stringify(payload));
  };

  if (!open) return null;
  return (
    <aside style={{
      position: "fixed", top: 0, right: 0, bottom: 0, width: 440, zIndex: 80,
      background: "var(--surface-sunken)", borderLeft: "1px solid var(--border-subtle)",
      boxShadow: "var(--shadow-3)", display: "flex", flexDirection: "column",
    }}>
      <header style={{
        flex: "0 0 auto", padding: "var(--space-6) var(--space-6) var(--space-5)",
        background: "var(--surface-inverse)", display: "flex", alignItems: "center", gap: "var(--space-5)",
      }}>
        <img src="../../assets/logo-mark-mint.svg" alt="" width="38" height="38" style={{ display: "block" }} />
        <div style={{ flex: 1, minWidth: 0 }}>
          <div style={{ font: "var(--type-heading-m)", color: "var(--sand-50)" }}>Ask Larkmere</div>
          <div style={{ display: "flex", alignItems: "center", gap: "var(--space-3)", marginTop: 2 }}>
            <span style={{ width: 7, height: 7, borderRadius: "var(--radius-pill)", background: "var(--mint-400)" }} />
            <span style={{ font: "var(--type-body-s)", color: "var(--green-200)" }}>Answers from your own accounts</span>
          </div>
        </div>
        <IconButton icon="x" label="Close" onClick={onClose} style={{ color: "var(--sand-50)" }} />
      </header>

      <div ref={scrollRef} style={{ flex: 1, overflowY: "auto", padding: "var(--space-6)", display: "flex", flexDirection: "column", gap: "var(--space-5)" }}>
        {thread.map((m, i) => (
          <div key={i}>
            <Bubble from={m.from}>{m.text}</Bubble>
            {m.card ? <ActionCard card={m.card} onToast={onToast} /> : null}
          </div>
        ))}
        {thinking ? (
          <Bubble from="bot">
            <span style={{ display: "inline-flex", gap: 4, alignItems: "center", color: "var(--text-muted)" }}>
              {[0, 1, 2].map((d) => (
                <span key={d} style={{ width: 6, height: 6, borderRadius: "var(--radius-pill)", background: "var(--ink-300)", opacity: 1 - d * 0.25 }} />
              ))}
              <span style={{ font: "var(--type-body-s)", marginLeft: 6 }}>Looking at your policies…</span>
            </span>
          </Bubble>
        ) : null}
      </div>

      {/* Suggested openers, and only before the member has said anything. Once
          the conversation has started the member types, like anyone would. */}
      {thread.length <= 1 ? (
        <div style={{ flex: "0 0 auto", padding: "0 var(--space-6) var(--space-5)", display: "flex", flexWrap: "wrap", gap: "var(--space-3)" }}>
          {OPENERS.map((o) => (
            <Chip key={o.label} label={o.label} onClick={() => ask(o.text)} />
          ))}
        </div>
      ) : null}

      {/* No borderTop. The thread above sits on --surface-sunken and this
          footer on --surface-card, so the surface change already separates
          them — and a rule here lands 17px above the composer's own top
          border, two panel-wide lines that read as one doubled edge. */}
      <footer style={{
        flex: "0 0 auto", padding: "var(--space-5) var(--space-6) var(--space-6)",
        background: "var(--surface-card)",
        display: "flex", flexDirection: "column", gap: "var(--space-4)",
      }}>
        <form onSubmit={(e) => { e.preventDefault(); ask(draft); }} style={{ display: "flex", gap: "var(--space-4)", alignItems: "center" }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <Input value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="Ask about your card" />
          </div>
          <IconButton icon="arrow-up" label="Send" variant="solid" onClick={() => ask(draft)} />
        </form>
        {/* Two separate statements, and conflating them would be a mistake.
            "Not advice" is a SUITABILITY disclaimer — the firm's own choice.
            The AI disclosure below is a LEGAL obligation under EU AI Act
            Art. 50(1), and it appears only because the pack declared it and
            the facade said so on connect. */}
        <div style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-3)" }}>
          <Badge tone="neutral">Not advice</Badge>
          <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", margin: 0 }}>
            An assistant, not an adviser. It never moves money or changes cover without your confirmation.
            <a href="#" onClick={(e) => { e.preventDefault(); onNavigate && onNavigate("claims"); }} style={{ marginLeft: 4 }}>Talk to a person</a>
          </p>
        </div>
        {disclosure ? (
          <div style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-3)" }}>
            <Badge tone="neutral">AI</Badge>
            <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", margin: 0 }}>
              You are chatting with an AI assistant, not a person.
            </p>
          </div>
        ) : null}
      </footer>
    </aside>
  );
}
