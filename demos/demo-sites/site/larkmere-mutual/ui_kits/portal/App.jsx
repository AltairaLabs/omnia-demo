import React, { useState, useEffect } from "react";
import { Rail, TopBar } from "./Chrome.jsx";
import { Dashboard } from "./Dashboard.jsx";
import { PolicyDetail } from "./PolicyDetail.jsx";
import { Claims } from "./Claims.jsx";
import { Transfer } from "./Transfer.jsx";
import { AssistantLauncher, AssistantPanel } from "./Assistant.jsx";

// The four screens the design project ships for this kit. PolicyDetail, Claims
// and Transfer arrived later than the rest: the original handoff was a
// "dashboard + assistant" bundle that shipped neither those screens nor the
// components they need (Field, Select, Radio, Checkbox, Switch, Tabs, Tag), so
// for a while importing them would have blanked the page on a module that never
// resolved. They and their components are now pulled, so the rail's four main
// destinations are all real.
//
// The secondary rail links (Documents, Retirement, Settings) have no screen in
// the design project at all. They render a titled empty state rather than
// silently bouncing back to the dashboard — a click that appears to do nothing
// reads as a broken page, which is the last thing you want one stray click away
// during a recording.
const EMPTY = {
  documents: ["Documents", "Your statements and policy schedules appear here as they are issued. Nothing new since 1 September."],
  retirement: ["Retirement", "Your plan is on track. The next projection is published in October."],
  settings: ["Settings", "Contact details, security and communication preferences."],
};

function Empty({ body }) {
  return (
    <div style={{
      background: "var(--surface-card)", border: "1px solid var(--border-subtle)",
      borderRadius: "var(--radius-lg)", boxShadow: "var(--shadow-1)",
      padding: "var(--space-9)", maxWidth: 620,
    }}>
      <p style={{ font: "var(--type-body)", color: "var(--text-muted)", margin: 0 }}>{body}</p>
    </div>
  );
}

const SCREENS = {
  dashboard: { title: "Your accounts", Screen: Dashboard },
  policy: { title: "Home & contents", Screen: PolicyDetail },
  claims: { title: "Claims", Screen: Claims },
  transfer: { title: "Move money", Screen: Transfer },
};

function App() {
  const [route, setRoute] = useState("dashboard");
  const [toast, setToast] = useState(null);
  const [assistant, setAssistant] = useState(false);
  const known = SCREENS[route];
  const [emptyTitle, emptyBody] = EMPTY[route] || EMPTY.settings;
  const title = known ? known.title : emptyTitle;
  useEffect(() => {
    const t = setTimeout(() => window.lucide && window.lucide.createIcons(), 40);
    return () => clearTimeout(t);
  }, [route, toast]);

  // The tab is in shot for the whole recording, so it says what a real bank's
  // tab says: the section you are on, then the brand. index.html's static
  // <title> is the pre-hydration fallback and stays as it is.
  useEffect(() => { document.title = title + " · Larkmere Mutual"; }, [title]);
  return (
    <div style={{ display: "flex", minHeight: "100vh", background: "var(--surface-page)" }}>
      <Rail route={route} onNavigate={setRoute} />
      <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
        <TopBar title={title} toast={toast} onDismissToast={() => setToast(null)} />
        <div style={{ padding: "var(--space-8) var(--space-9) var(--space-13)" }}>
          {known
            ? <known.Screen onNavigate={setRoute} onToast={setToast} />
            : <Empty body={emptyBody} />}
        </div>
      </div>
      <AssistantLauncher open={assistant} onClick={() => setAssistant(true)} />
      <AssistantPanel open={assistant} onClose={() => setAssistant(false)} onToast={setToast} onNavigate={setRoute} />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
