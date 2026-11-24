import React, { useState, useEffect, useRef } from "react";

// Ellen's assistant: the PERSON's side of the Poppy demo, laid out like a
// phone app. Unbranded: it is Ellen's own assistant, not a fourth company,
// and it looks like no real assistant.
//
// STAND-IN (docs/stand-ins.md, "Personal Agent"). The page has no sign-in:
// whoever can open it (behind the dashboard's sign-in, via the catalogue) acts
// as Ellen. The site server (demos/demo-sites/server.go) is the edge: it
// asserts Ellen to the assistant agent with a token it signs for her, and it
// forwards nothing of the browser's identity anywhere.
//
// Three parts, all through the site server, at URLs relative to this page:
//
//   THE CHAT. api/assistant/ws, the assistant agent's websocket facade. Ellen
//   talks to her assistant; it talks to the companies with its tools.
//
//   THE STRIP. One row per company, with the latest state of the assistant's
//   conversation there. api/conversations is the snapshot (so a reload is not
//   empty), then api/updates (server-sent events from personal-agent) keeps
//   it current between turns, including replies that arrive later.
//
//   TALK TO THEM YOURSELF. Ellen's own lines go to api/direct, which opens a
//   Direct Conversation with the company (PAP §8.10) on first use. The
//   company's replies come back on api/updates, tagged with the parent.
//
//   SIGN IN. When a company wants Ellen signed in, its row says so. Once the
//   assistant starts a sign-in (its sign_in tool), personal-agent publishes
//   {"type":"signin","status":"pending","authorization_url":...} on
//   api/updates and the row shows a "Sign in to <Company>" button. Only
//   Ellen's click opens it, in a new tab: she signs in on the company's own
//   page, which redirects to personal-agent's /signin/callback (never this
//   site). How it ended comes back as another "signin" update, and the page
//   tells the assistant in Ellen's words. No token, code or state ever
//   reaches this page; the URL carries only the state and code challenge.

const OPENER = "The storm's flooded the kitchen and the power's off. Sort it out.";

function url(path) {
  return new URL("../../api/" + path, window.location.href);
}

function socketURL() {
  const u = url("assistant/ws");
  u.protocol = u.protocol === "https:" ? "wss:" : "ws:";
  return u.href;
}

// The model writes **bold** now and then; show it as bold, and nothing else
// as markup.
function emphasis(text) {
  return String(text || "").split(/\*\*([^*]+)\*\*/g).map((part, i) => (i % 2 ? <strong key={i}>{part}</strong> : part));
}

// httpsURL is u when it is an https URL, else "": a sign-in page is only
// ever opened over https.
function httpsURL(u) {
  try {
    const x = new URL(String(u || ""));
    return x.protocol === "https:" ? x.href : "";
  } catch (e) {
    return "";
  }
}

// Sign-in lines the page sends the assistant for Ellen.
const signedInLine = (company) => "I've signed in to " + company + ".";
const declinedLine = (company) => "I didn't sign in to " + company + ".";

// ---------------------------------------------------------------------------
// Conversations: what the strip knows, by conversation id.
//
//   { id, company, parent, working, handoff, closed, stopped, needsYou,
//     signin: null | { needed, status, url },
//     messages: [{ role, author, text }], seen: { eventId: true } }
//
// role is "user" for the person's side (the assistant, or Ellen herself in a
// Direct Conversation: author "human") and "company" for the company's.
//
// signin.status is needed, pending, signed_in, failed or declined. url is
// the authorization URL while pending, and only from an update: the snapshot
// does not carry it, so after a reload the assistant starts the sign-in again.

function fromSnapshot(c) {
  const si = c.sign_in;
  return {
    id: c.conversation_id, company: c.company, parent: c.parent_conversation_id || "",
    working: !!c.working, handoff: c.handoff || "none", closed: !!c.closed, stopped: c.stopped || "",
    needsYou: false, messages: (c.messages || []).map((m) => ({ role: m.role, author: m.author, text: m.text })),
    signin: si ? { needed: !!si.needed, status: si.status || "", url: "" } : null,
    seen: {}, snapshot: true,
  };
}

// stoppedForSignIn: the company refused the assistant until Ellen signs in
// (personal-agent's reason "sign_in_required ...", PAP §5.4).
function stoppedForSignIn(c) {
  return c.stopped.startsWith("sign_in_required");
}

// wantsSignIn: the company wants Ellen signed in and she hasn't, or a
// sign-in has begun and not come back.
function wantsSignIn(c) {
  const s = c.signin;
  if (!s) return false;
  return s.status === "pending" || s.status === "needed" || (s.status === "failed" && s.needed);
}

// apply folds one update into the conversations and returns the new map.
// During the replay after a snapshot (replay true), a message the snapshot
// already holds is skipped: the snapshot carries no event ids to match on.
function apply(convs, u, replay) {
  const id = u.conversation_id;
  const ev = u.event || {};
  const prev = convs[id] || {
    id, company: u.company, parent: u.parent_conversation_id || "", working: false, handoff: "none",
    closed: false, stopped: "", needsYou: false, signin: null, messages: [], seen: {},
  };
  if (ev.id && prev.seen[ev.id]) return convs;
  const c = { ...prev, company: u.company || prev.company, seen: ev.id ? { ...prev.seen, [ev.id]: true } : prev.seen };
  switch (ev.type) {
    case "message": {
      const m = ev.message || {};
      const msg = { role: m.role, author: (m.author && m.author.type) || "", text: m.text || "" };
      if (replay && c.snapshot && c.messages.some((x) => x.role === msg.role && x.text === msg.text)) return convs;
      c.messages = [...c.messages, msg];
      // Anything from the person's side answers what the company asked for,
      // a sign-in included, unless the company stopped talking until then.
      if (msg.role !== "company") {
        c.needsYou = false;
        if (c.signin && c.signin.status === "needed" && !stoppedForSignIn(c)) c.signin = null;
      }
      break;
    }
    case "working":
      c.working = !!ev.working;
      break;
    case "state":
      if (typeof ev.working === "boolean") c.working = ev.working;
      if (ev.handoff) c.handoff = ev.handoff;
      if (typeof ev.closed === "boolean") c.closed = ev.closed;
      break;
    case "user_requested":
      c.needsYou = true;
      break;
    case "authorization": {
      // The company wants Ellen signed in (§8.11). A sign-in already begun
      // stays pending, with its button.
      const s = c.signin || { url: "" };
      c.needsYou = true;
      c.signin = { ...s, needed: true, status: s.status === "pending" ? "pending" : "needed" };
      break;
    }
    case "signin": {
      const status = String(ev.status || "");
      const s = c.signin || { needed: false };
      c.signin = { needed: status === "signed_in" ? false : s.needed, status, url: status === "pending" ? httpsURL(ev.authorization_url) : "" };
      if (status === "signed_in" || status === "declined") c.needsYou = false;
      // Signed in, personal-agent follows a conversation stopped for it again.
      if (status === "signed_in" && stoppedForSignIn(c)) c.stopped = "";
      break;
    }
    case "stopped":
      c.stopped = ev.reason || "stopped";
      c.working = false;
      break;
    default:
      return convs; // direct_opened and anything newer: nothing to show
  }
  return { ...convs, [id]: c };
}

// state is a conversation's row label: working, replied, waiting for you,
// needs you to sign in, done or stopped. "Waiting for you" is only for what
// really needs Ellen (the company asked for her); a company having spoken
// last is just "Replied", since the assistant may well answer it.
function state(c) {
  if (c.stopped && !stoppedForSignIn(c)) return { key: "stopped", label: "Stopped" };
  if (c.closed) return { key: "done", label: "Done" };
  if (wantsSignIn(c)) return { key: "signin", label: "Needs you to sign in" };
  if (c.stopped) return { key: "stopped", label: "Stopped" };
  if (c.needsYou) return { key: "waiting", label: "Waiting for you" };
  if (c.working) return { key: "working", label: c.handoff === "pending" ? "Passing you to a person" : "Working" };
  const last = c.messages[c.messages.length - 1];
  if (last && last.role === "company") return { key: "replied", label: "Replied" };
  return { key: "working", label: "Working" };
}

function lastLine(c) {
  if (c.stopped && stoppedForSignIn(c)) {
    return c.signin && c.signin.status === "declined"
      ? "You didn't sign in to " + c.company
      : "Waiting until you sign in to " + c.company;
  }
  if (c.stopped) return c.stopped;
  const last = c.messages[c.messages.length - 1];
  if (!last) return "Starting the conversation";
  const who = last.role === "company" ? "" : last.author === "human" ? "You: " : "Your assistant: ";
  return who + last.text;
}

// rows is one row per company: its most recent conversation that is not a
// Direct Conversation, with that conversation's Direct Conversation if any.
function rows(convs, order) {
  const byCompany = {};
  for (const id of order) {
    const c = convs[id];
    if (c && !c.parent) byCompany[c.company] = c;
  }
  return Object.values(byCompany).map((c) => {
    let direct = null;
    for (const id of order) {
      const d = convs[id];
      if (d && d.parent === c.id) direct = d;
    }
    return { conv: c, direct };
  });
}

// ---------------------------------------------------------------------------

// useConversations follows the strip. onSignIn(company, status) hears every
// "signin" update as it arrives on the stream (never from a snapshot), once
// per conversation it was published on.
function useConversations(onSignIn) {
  const [convs, setConvs] = useState({});
  const [order, setOrder] = useState([]);
  const signInRef = useRef(onSignIn);
  signInRef.current = onSignIn;

  useEffect(() => {
    let buffer = null; // updates that arrive while a snapshot is loading
    let gen = 0; // the latest load wins
    let closed = false;
    const remember = (id) => setOrder((o) => (o.includes(id) ? o : [...o, id]));

    const load = async () => {
      const mine = ++gen;
      buffer = buffer || [];
      try {
        const res = await fetch(url("conversations"), { headers: { Accept: "application/json" } });
        if (!res.ok) throw new Error(String(res.status));
        const list = await res.json();
        if (closed || mine !== gen) return;
        const next = {};
        for (const c of list) next[c.conversation_id] = fromSnapshot(c);
        const pending = buffer;
        buffer = null;
        setConvs(() => pending.reduce((acc, u) => apply(acc, u, true), next));
        setOrder(() => {
          const ids = list.map((c) => c.conversation_id);
          for (const u of pending) if (!ids.includes(u.conversation_id)) ids.push(u.conversation_id);
          return ids;
        });
      } catch (e) {
        // No snapshot: follow the stream alone, from now.
        if (closed || mine !== gen) return;
        const pending = buffer || [];
        buffer = null;
        setConvs((cur) => pending.reduce((acc, u) => apply(acc, u, false), cur));
        pending.forEach((u) => remember(u.conversation_id));
      }
    };

    load();
    const es = new EventSource(url("updates"));
    // Every (re)connect reloads the snapshot too: whatever was missed while
    // the stream was down is in it.
    es.onopen = () => { load(); };
    es.onmessage = (ev) => {
      let u;
      try { u = JSON.parse(ev.data); } catch (e) { return; }
      if (!u || !u.conversation_id) return;
      if (u.event && u.event.type === "signin" && u.company) signInRef.current(String(u.company), String(u.event.status || ""));
      if (buffer) { buffer.push(u); return; }
      setConvs((cur) => apply(cur, u, false));
      remember(u.conversation_id);
    };
    return () => { closed = true; es.close(); };
  }, []);

  return { convs, order };
}

// ---------------------------------------------------------------------------

function Bubble({ m, company }) {
  if (m.role === "company") {
    return <div className="ea-msg it"><span className="ea-who">{company}</span>{emphasis(m.text)}</div>;
  }
  const who = m.author === "human" ? "You" : "Your assistant";
  return <div className="ea-msg me"><span className="ea-who">{who}</span>{emphasis(m.text)}</div>;
}

// TalkYourself is the sheet for one company: what the assistant and the
// company said, then Ellen's own Direct Conversation with them.
function TalkYourself({ row, onClose }) {
  const { conv, direct } = row;
  const [draft, setDraft] = useState("");
  const [sending, setSending] = useState(false);
  const [pending, setPending] = useState([]); // Ellen's lines not yet seen in the stream
  const [error, setError] = useState("");
  const scrollRef = useRef(null);

  const said = direct ? direct.messages : [];
  // A sent line shows at once, until the company's stream carries it back.
  const unseen = pending.filter((t) => !said.some((m) => m.role !== "company" && m.text === t));

  useEffect(() => {
    const box = scrollRef.current;
    if (box) box.scrollTop = box.scrollHeight;
  }, [said.length, unseen.length]);

  const send = async () => {
    const text = draft.trim();
    if (!text || sending) return;
    setDraft("");
    setError("");
    setSending(true);
    setPending((p) => [...p, text]);
    try {
      const res = await fetch(url("direct"), {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify({ conversation_id: live ? direct.id : conv.id, text }),
      });
      if (!res.ok) {
        let msg = "Couldn't send that to " + conv.company + ".";
        try { const b = await res.json(); if (b.error) msg = b.error; } catch (e) { /* keep the plain message */ }
        throw new Error(msg);
      }
    } catch (e) {
      setPending((p) => p.filter((t) => t !== text));
      setError(e.message);
    } finally {
      setSending(false);
    }
  };

  // A closed Direct Conversation is not the end: the next line opens a new
  // one, as long as the assistant's conversation it hangs off is still open.
  const live = direct && !direct.closed && !direct.stopped;
  const ended = !live && !!(conv.closed || conv.stopped);

  return (
    <div className="ea-sheet-back" onClick={onClose}>
      <div className="ea-sheet" role="dialog" aria-label={"Talk to " + conv.company + " yourself"} onClick={(e) => e.stopPropagation()}>
        <div className="ea-sheet-head">
          <h2>{conv.company}</h2>
          {direct && <span className={"ea-state " + state(direct).key}>{state(direct).label}</span>}
          <button className="ea-link ea-close" onClick={onClose}>Close</button>
        </div>
        <div className="ea-chat" ref={scrollRef}>
          <div className="ea-past">
            {conv.messages.map((m, i) => <Bubble key={i} m={m} company={conv.company} />)}
          </div>
          <div className="ea-divider">You, talking to {conv.company} yourself</div>
          {said.length === 0 && unseen.length === 0 && (
            <div className="ea-msg note">Your first message opens a conversation between you and {conv.company}. Your assistant doesn't speak in it.</div>
          )}
          {said.map((m, i) => <Bubble key={i} m={m} company={conv.company} />)}
          {unseen.map((t, i) => <div key={"p" + i} className="ea-msg me"><span className="ea-who">You</span>{t}</div>)}
          {direct && direct.working && <div className="ea-typing">{conv.company} is writing…</div>}
          {error && <div className="ea-msg note">{error}</div>}
        </div>
        <form className="ea-compose" onSubmit={(e) => { e.preventDefault(); send(); }}>
          <input value={draft} onChange={(e) => setDraft(e.target.value)} disabled={ended}
            placeholder={ended ? "This conversation has ended" : "Write to " + conv.company} aria-label={"Message to " + conv.company} />
          <button className="ea-send" disabled={ended || sending || !draft.trim()}>Send</button>
        </form>
      </div>
    </div>
  );
}

// SignInButton opens the company's own sign-in page in a new tab, on Ellen's
// click only, and only over https. noopener: the company's page gets no
// handle on this one.
function SignInButton({ conv }) {
  const href = httpsURL(conv.signin && conv.signin.url);
  if (!href || conv.signin.status !== "pending") return null;
  return (
    <button className="ea-link ea-signin" onClick={() => { const u = httpsURL(href); if (u) window.open(u, "_blank", "noopener"); }}>
      {"Sign in to " + conv.company}
    </button>
  );
}

function Strip({ list, onTake }) {
  if (list.length === 0) {
    return <div className="ea-strip-empty">No conversations yet. When your assistant contacts a company, it shows here.</div>;
  }
  return (
    <div className="ea-strip" aria-label="Conversations">
      {list.map((r) => {
        const s = state(r.conv);
        const shown = r.direct && !r.direct.closed ? lastLine(r.direct) : lastLine(r.conv);
        return (
          <div className="ea-row" key={r.conv.id}>
            <div>
              <span className="ea-co">{r.conv.company}</span><span className={"ea-state " + s.key}>{s.label}</span>
              {r.conv.signin && r.conv.signin.status === "signed_in" && <span className="ea-state signed">Signed in</span>}
            </div>
            <div className="ea-last" title={shown}>{shown}</div>
            <SignInButton conv={r.conv} />
            <button className="ea-link ea-take" onClick={() => onTake(r.conv.id)}
              disabled={!r.direct && !!(r.conv.closed || r.conv.stopped)}>
              {r.direct ? "Your conversation" : "Talk to them yourself"}
            </button>
          </div>
        );
      })}
    </div>
  );
}

function App() {
  // The latest sign-in status per company, as the stream told it. A sign-in
  // is published on every open conversation with the company; only its first
  // copy is said to the assistant.
  const signInsRef = useRef({});
  const onSignIn = (company, status) => {
    const was = signInsRef.current[company];
    signInsRef.current[company] = status;
    if (was === status) return;
    if (status === "signed_in") say(signedInLine(company));
    else if (status === "declined") say(declinedLine(company));
  };
  const { convs, order } = useConversations(onSignIn);
  const [thread, setThread] = useState([
    { from: "it", text: "Hi Ellen. Tell me what you need and I'll get in touch with the companies you deal with. I'll tell you what each one says, and I'll ask you before agreeing to anything." },
  ]);
  const [draft, setDraft] = useState("");
  const [busy, setBusyState] = useState(false);
  const busyRef = useRef(false);
  const queueRef = useRef([]); // Ellen's sign-in lines, held while a turn runs
  const setBusy = (b) => { busyRef.current = b; setBusyState(b); };
  const [offline, setOffline] = useState(false);
  const [open, setOpen] = useState(null); // conversation id of the sheet's row
  const sockRef = useRef(null);
  const sessionRef = useRef(null);
  const turnRef = useRef("");
  const scrollRef = useRef(null);

  useEffect(() => {
    const sock = new WebSocket(socketURL());
    sockRef.current = sock;
    const send = (m) => {
      if (sessionRef.current) m.session_id = sessionRef.current;
      if (sock.readyState === WebSocket.OPEN) sock.send(JSON.stringify(m));
    };
    sock.onopen = () => setOffline(false);
    sock.onmessage = (ev) => {
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      switch (msg.type) {
        case "connected":
          sessionRef.current = msg.session_id || null;
          return;
        case "chunk":
          turnRef.current += msg.content || "";
          return;
        case "tool_call": {
          // The assistant has no client tools; its tools run server-side. Ack
          // then refuse anything that arrives, rather than let it time out.
          const call = msg.tool_call || {};
          send({ type: "tool_call_ack", tool_call_ack: { call_id: call.id } });
          send({ type: "tool_call_nack", tool_call_nack: { call_id: call.id, reason: "This page runs no tools." } });
          return;
        }
        case "done": {
          const text = (msg.content || turnRef.current).trim();
          turnRef.current = "";
          if (text) setThread((t) => [...t, { from: "it", text }]);
          setBusy(false);
          flush();
          return;
        }
        case "error":
          turnRef.current = "";
          setBusy(false);
          setThread((t) => [...t, { from: "it", text: "Sorry, something went wrong on my side. Try again in a moment." }]);
          flush();
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

  // sendLine sends Ellen's line to the assistant as a turn. It reads refs
  // only, so the socket's handlers may call it from their first render.
  function sendLine(text) {
    setThread((t) => [...t, { from: "me", text }]);
    const sock = sockRef.current;
    if (!sock || sock.readyState !== WebSocket.OPEN) {
      setThread((t) => [...t, { from: "note", text: "Not connected. Reload the page to try again." }]);
      return;
    }
    setBusy(true);
    const m = { type: "message", content: text };
    if (sessionRef.current) m.session_id = sessionRef.current;
    sock.send(JSON.stringify(m));
  }

  // say sends a line for Ellen now, or after the turn that is running.
  function say(text) {
    if (busyRef.current) queueRef.current.push(text);
    else sendLine(text);
  }

  function flush() {
    const next = queueRef.current.shift();
    if (next) sendLine(next);
  }

  const ask = (q) => {
    const text = q.trim();
    if (!text || busy) return;
    setDraft("");
    sendLine(text);
  };

  const list = rows(convs, order);
  const sheet = open ? list.find((r) => r.conv.id === open) : null;

  return (
    <div className="ea-stage">
      <div className="ea-phone">
        <div className="ea-head">
          <h1>Ellen's assistant</h1>
          <span className="ea-for">for Ellen Whitcomb</span>
          <span className={"ea-dot" + (offline ? " off" : "")} title={offline ? "Not connected" : "Connected"} />
        </div>
        <Strip list={list} onTake={setOpen} />
        <div className="ea-chat" ref={scrollRef}>
          {thread.map((m, i) => (
            <div key={i} className={"ea-msg " + m.from}>{m.from === "note" ? m.text : emphasis(m.text)}</div>
          ))}
          {busy && <div className="ea-typing">Working on it…</div>}
        </div>
        {thread.length === 1 && (
          <div className="ea-suggest">
            <button className="ea-link" onClick={() => ask(OPENER)}>{OPENER}</button>
          </div>
        )}
        <form className="ea-compose" onSubmit={(e) => { e.preventDefault(); ask(draft); }}>
          <input value={draft} onChange={(e) => setDraft(e.target.value)} placeholder="Ask your assistant" aria-label="Message to your assistant" />
          <button className="ea-send" disabled={busy || !draft.trim()}>Send</button>
        </form>
        {sheet && <TalkYourself row={sheet} onClose={() => setOpen(null)} />}
      </div>
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
