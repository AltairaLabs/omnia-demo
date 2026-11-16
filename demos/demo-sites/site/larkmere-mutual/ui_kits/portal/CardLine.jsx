import React, { useState, useEffect, useRef } from "react";
import { Button } from "../../components/core/Button.jsx";
import { Badge } from "../../components/core/Badge.jsx";
import { Icon } from "../../components/core/Icon.jsx";

// The card line: a voice call to Larkmere's realtime voice agent (card-line),
// from inside the signed-in portal.
//
// The portal proxies the socket at api/phone/ws exactly as it does the chat
// (assistantProxy in demos/demo-sites/server.go), so the call carries the same
// verified member and this page holds no credential. Audio crosses the socket
// as OMNI binary frames, the framing Omnia's own dashboard console uses
// (dashboard/src/lib/omni-binary.ts): a 32-byte header, JSON metadata, then
// 16-bit little-endian mono PCM.
//
// Nothing here decides anything. The disclosure the caller hears first is
// platform audio the runtime plays because the pack requires it; which card
// the agent can touch is the card system's decision; whether a tool may run is
// the ToolPolicy's. This is a microphone and a speaker.

const HEADER = 32;
const FRAME_MS = 20;
const FLAG_LAST = 4;

// The capture worklet: Float32 render quanta from the mic, as Int16 PCM.
const CAPTURE_WORKLET = `
class PcmCapture extends AudioWorkletProcessor {
  process(inputs) {
    const ch = inputs[0] && inputs[0][0];
    if (ch && ch.length) {
      const out = new Int16Array(ch.length);
      for (let i = 0; i < ch.length; i++) {
        const s = Math.max(-1, Math.min(1, ch[i]));
        out[i] = s < 0 ? s * 0x8000 : s * 0x7fff;
      }
      this.port.postMessage(out.buffer, [out.buffer]);
    }
    return true;
  }
}
registerProcessor("pcm-capture", PcmCapture);
`;

function omniFrame(sessionId, seq, payload, sampleRate, last) {
  const meta = new TextEncoder().encode(JSON.stringify({
    session_id: sessionId, mime_type: "audio/pcm", sample_rate: sampleRate, channels: 1, codec: "pcm",
  }));
  const buf = new ArrayBuffer(HEADER + meta.length + payload.byteLength);
  const v = new DataView(buf);
  [0x4f, 0x4d, 0x4e, 0x49].forEach((b, i) => v.setUint8(i, b)); // "OMNI"
  v.setUint8(4, 1); // version
  v.setUint8(5, last ? FLAG_LAST : 0);
  v.setUint16(6, 1); // audio
  v.setUint32(8, meta.length);
  v.setUint32(12, payload.byteLength);
  v.setUint32(16, seq >>> 0);
  new Uint8Array(buf, HEADER, meta.length).set(meta);
  new Uint8Array(buf, HEADER + meta.length).set(new Uint8Array(payload));
  return buf;
}

function omniPayload(data) {
  const v = new DataView(data);
  const metaLen = v.getUint32(8);
  const len = v.getUint32(12);
  return new Int16Array(data.slice(HEADER + metaLen, HEADER + metaLen + len));
}

const STATUS = {
  dialling: "Calling the card line…",
  live: "On the call",
  ended: "Call ended",
  failed: "The card line isn't available right now.",
  nomic: "Larkmere needs your microphone for the call.",
};

export function CardLineCall({ onEnd }) {
  const [status, setStatus] = useState("dialling");
  const [lines, setLines] = useState([]);
  const [disclosure, setDisclosure] = useState(false);
  const [speaking, setSpeaking] = useState(false);
  const scrollRef = useRef(null);
  const endRef = useRef(() => {});

  useEffect(() => {
    let sock = null;
    let ctx = null;
    let stream = null;
    let sessionId = null;
    let seq = 0;
    let rate = 24000;
    let micStarted = false;
    let playAt = 0;
    let playing = [];
    let pending = new Int16Array(0);
    let agentLine = null; // the agent's turn being transcribed
    let closed = false;

    const addLine = (from, text) => setLines((ls) => [...ls, { from, text }]);
    const setAgent = (text) => setLines((ls) => {
      const copy = ls.slice();
      if (agentLine === null) { agentLine = copy.length; copy.push({ from: "line", text }); }
      else copy[agentLine] = { from: "line", text };
      return copy;
    });
    let agentText = "";

    const flushPlayback = () => {
      playing.forEach((src) => { try { src.stop(); } catch (e) { /* already ended */ } });
      playing = [];
      if (ctx) playAt = ctx.currentTime;
      setSpeaking(false);
    };

    const play = (pcm) => {
      if (!ctx || !pcm.length) return;
      const buf = ctx.createBuffer(1, pcm.length, rate);
      const ch = buf.getChannelData(0);
      for (let i = 0; i < pcm.length; i++) ch[i] = pcm[i] / 0x8000;
      const src = ctx.createBufferSource();
      src.buffer = buf;
      src.connect(ctx.destination);
      const at = Math.max(ctx.currentTime + 0.05, playAt);
      src.start(at);
      playAt = at + buf.duration;
      playing.push(src);
      setSpeaking(true);
      src.onended = () => {
        playing = playing.filter((s) => s !== src);
        if (!playing.length) setSpeaking(false);
      };
    };

    const startMic = async () => {
      if (micStarted || closed) return;
      micStarted = true;
      try {
        stream = await navigator.mediaDevices.getUserMedia({
          audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
        });
      } catch (e) {
        setStatus("nomic");
        end(false);
        return;
      }
      ctx = new AudioContext({ sampleRate: rate });
      playAt = ctx.currentTime;
      const url = URL.createObjectURL(new Blob([CAPTURE_WORKLET], { type: "text/javascript" }));
      await ctx.audioWorklet.addModule(url);
      URL.revokeObjectURL(url);
      const node = new AudioWorkletNode(ctx, "pcm-capture");
      const frame = Math.round(rate * FRAME_MS / 1000);
      node.port.onmessage = (ev) => {
        const chunk = new Int16Array(ev.data);
        const joined = new Int16Array(pending.length + chunk.length);
        joined.set(pending); joined.set(chunk, pending.length);
        let off = 0;
        while (joined.length - off >= frame) {
          const out = joined.slice(off, off + frame);
          off += frame;
          if (sock && sock.readyState === WebSocket.OPEN && sessionId) {
            sock.send(omniFrame(sessionId, seq++, out.buffer, rate, false));
          }
        }
        pending = joined.slice(off);
      };
      ctx.createMediaStreamSource(stream).connect(node);
      setStatus("live");
    };

    const end = (hangup) => {
      if (closed) return;
      closed = true;
      if (hangup && sock && sock.readyState === WebSocket.OPEN) {
        try { sock.send(JSON.stringify({ type: "hangup", session_id: sessionId })); } catch (e) { /* closing anyway */ }
      }
      if (sock) sock.close();
      if (stream) stream.getTracks().forEach((t) => t.stop());
      flushPlayback();
      if (ctx) ctx.close();
    };
    endRef.current = () => { end(true); setStatus("ended"); };

    // Relative to this page, so it works under a path prefix.
    const url = new URL("../../api/phone/ws?binary=true", window.location.href);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    sock = new WebSocket(url);
    sock.binaryType = "arraybuffer";

    sock.onmessage = (ev) => {
      if (typeof ev.data !== "string") { play(omniPayload(ev.data)); return; }
      let msg;
      try { msg = JSON.parse(ev.data); } catch (e) { return; }
      switch (msg.type) {
        case "connected":
          sessionId = msg.session_id || null;
          // Read from the wire, as the chat's notice is: the pack declares it.
          setDisclosure((msg.connected || {}).ai_disclosure === true);
          // The runtime counter-offers its capture format in session_config;
          // if none comes, it takes ours.
          setTimeout(startMic, 1500);
          break;
        case "session_config":
          if (msg.session_config && msg.session_config.sample_rate) rate = msg.session_config.sample_rate;
          startMic();
          break;
        case "interrupt":
          // The caller talked over the agent: stop speaking now.
          flushPlayback();
          break;
        case "chunk":
          if (msg.role === "user") {
            addLine("member", msg.content || "");
          } else {
            agentText += msg.content || "";
            setAgent(agentText);
          }
          break;
        case "done":
          agentLine = null;
          agentText = "";
          break;
        case "error":
          setStatus("failed");
          end(false);
          break;
        default:
      }
    };
    sock.onerror = () => { if (!closed) { setStatus("failed"); end(false); } };
    sock.onclose = () => { if (!closed) { setStatus((s) => (s === "live" ? "ended" : "failed")); end(false); } };

    return () => end(true);
  }, []);

  React.useLayoutEffect(() => {
    const box = scrollRef.current;
    if (box) box.scrollTop = box.scrollHeight;
    const t = setTimeout(() => window.lucide && window.lucide.createIcons(), 40);
    return () => clearTimeout(t);
  }, [lines, status]);

  const over = status === "ended" || status === "failed" || status === "nomic";

  return (
    <div style={{ flex: 1, display: "flex", flexDirection: "column", minHeight: 0 }}>
      <div style={{
        flex: "0 0 auto", padding: "var(--space-8) var(--space-6) var(--space-6)",
        display: "flex", flexDirection: "column", alignItems: "center", gap: "var(--space-4)",
      }}>
        <div style={{
          width: 88, height: 88, borderRadius: "var(--radius-pill)",
          display: "flex", alignItems: "center", justifyContent: "center",
          background: over ? "var(--surface-card)" : "var(--green-900)",
          color: over ? "var(--text-muted)" : "var(--mint-400)",
          boxShadow: speaking ? "0 0 0 10px color-mix(in oklab, var(--mint-400) 25%, transparent)" : "var(--shadow-1)",
          transition: "box-shadow var(--dur-base) var(--ease-standard)",
        }}>
          <Icon name={over ? "phone-off" : "phone"} size={34} />
        </div>
        <div style={{ font: "var(--type-heading-m)", color: "var(--text-primary)" }}>Larkmere card line</div>
        <div style={{ font: "var(--type-body-s)", color: "var(--text-secondary)" }}>{STATUS[status]}</div>
      </div>

      <div ref={scrollRef} style={{ flex: 1, overflowY: "auto", padding: "0 var(--space-6) var(--space-6)", display: "flex", flexDirection: "column", gap: "var(--space-3)" }}>
        {lines.map((l, i) => (
          <div key={i} style={{ font: "var(--type-body-s)", lineHeight: 1.55, color: l.from === "member" ? "var(--text-secondary)" : "var(--text-primary)" }}>
            <span style={{ font: "var(--type-eyebrow)", letterSpacing: "var(--tracking-caps)", textTransform: "uppercase", color: "var(--text-muted)", marginRight: 8 }}>
              {l.from === "member" ? "You" : "Larkmere"}
            </span>
            {l.text}
          </div>
        ))}
      </div>

      <footer style={{
        flex: "0 0 auto", padding: "var(--space-5) var(--space-6) var(--space-6)",
        background: "var(--surface-card)", display: "flex", flexDirection: "column", gap: "var(--space-4)",
      }}>
        {over ? (
          <Button variant="secondary" onClick={onEnd}>Back to chat</Button>
        ) : (
          <Button variant="danger" onClick={() => { endRef.current(); }}>
            <Icon name="phone-off" size={18} />End call
          </Button>
        )}
        {disclosure ? (
          <div style={{ display: "flex", alignItems: "flex-start", gap: "var(--space-3)" }}>
            <Badge tone="neutral">AI</Badge>
            <p style={{ font: "var(--type-body-s)", color: "var(--text-muted)", margin: 0 }}>
              You are speaking with an AI assistant, not a person. It tells you so when the call starts.
            </p>
          </div>
        ) : null}
      </footer>
    </div>
  );
}
