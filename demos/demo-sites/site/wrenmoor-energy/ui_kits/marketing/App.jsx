import React from "react";
import { Logo, Disclaimer, Badge, LiveDot, MixBar, MixLegend, AgentAvatar, UserBubble, AgentReceipt } from "../../components/Brand.jsx";
import { MIX, TARIFFS, SITES } from "../../data/account.js";

// Wrenmoor Energy's public website (the handoff's "Wrenmoor Website").
// Links between the pages are relative: Sign in -> the customer portal,
// Staff login -> the intranet.
const PORTAL = "../portal/";
const INTRANET = "../intranet/";
const PHOTOS = "../../assets/photos/";

const wrap = { maxWidth: 1280, width: "100%", margin: "0 auto" };

function Header() {
  return (
    <header style={{ ...wrap, padding: "22px 32px", display: "flex", alignItems: "center", justifyContent: "space-between", gap: 24, flexWrap: "wrap" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 40, flexWrap: "wrap" }}>
        <Logo href="./" />
        <nav style={{ display: "flex", gap: 28, fontSize: 15, flexWrap: "wrap" }} aria-label="Main">
          <a className="navlink" href="#tariffs">Home energy</a>
          <a className="navlink" href="#tariffs">Business</a>
          <a className="navlink" href="#sites">Our wind &amp; solar</a>
          <a className="navlink" href="#wren">Help</a>
        </nav>
      </div>
      <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
        <a className="btn btn-outline" href={PORTAL} style={{ padding: "10px 18px" }}>Sign in</a>
        <a className="btn btn-primary" href="#tariffs" style={{ padding: "11px 20px" }}>Get a quote</a>
      </div>
    </header>
  );
}

function Hero() {
  return (
    <section style={{ ...wrap, padding: "48px 32px 32px", display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,440px),1fr))", gap: 48, alignItems: "center" }}>
      <div style={{ display: "flex", flexDirection: "column", gap: 28 }}>
        <h1 className="display">Power from the moor, straight to your door.</h1>
        <p className="body-copy" style={{ margin: 0, fontSize: 19, lineHeight: 1.55, maxWidth: "46ch", textWrap: "pretty" }}>
          We generate our own wind and solar power in the South Pennines and sell it directly to homes and businesses. Fair prices, real people, and you can see where every kilowatt-hour came from.
        </p>
        <form onSubmit={(e) => e.preventDefault()} style={{ display: "flex", gap: 8, flexWrap: "wrap", maxWidth: 460 }}>
          <input className="input input-pill" aria-label="Your postcode" placeholder="Your postcode" style={{ flex: 1, minWidth: 180 }} />
          <button type="submit" className="btn btn-primary" style={{ fontSize: 16, padding: "14px 24px" }}>See my price</button>
        </form>
        <span className="muted" style={{ fontSize: 14 }}>Switching takes 5 minutes. We handle everything with your old supplier.</span>
      </div>
      <div style={{ position: "relative", minHeight: 520, borderRadius: "var(--r-hero)", background: "var(--photo-bg) url('" + PHOTOS + "hero-moor.jpg') center/cover no-repeat" }}
        role="img" aria-label="Wind turbines on a moor at sunset">
        <div className="float" style={{ position: "absolute", left: 24, right: 24, bottom: 24, background: "var(--card)", borderRadius: "var(--r-card)", padding: "22px 24px", display: "flex", flexDirection: "column", gap: 14, boxShadow: "var(--float)" }}>
          <div style={{ display: "flex", justifyContent: "space-between", alignItems: "baseline", gap: 12, flexWrap: "wrap" }}>
            <span className="muted" style={{ fontSize: 14, display: "flex", alignItems: "center", gap: 8 }}><LiveDot />Live from our sites · {MIX.at}</span>
            <span className="mono" style={{ fontSize: 13 }}>{MIX.totalMW}</span>
          </div>
          <MixBar parts={MIX.parts} />
          <MixLegend parts={MIX.parts} />
        </div>
      </div>
    </section>
  );
}

function Stat({ big, text }) {
  return (
    <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
      <span className="big" style={{ fontSize: 56 }}>{big}</span>
      <span className="body-copy" style={{ fontSize: 16, lineHeight: 1.5 }}>{text}</span>
    </div>
  );
}

function Stats() {
  return (
    <section style={{ ...wrap, padding: "72px 32px", display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,300px),1fr))", gap: 40, borderTop: "1px solid var(--line)" }}>
      <Stat big="38,400" text="homes and 1,200 businesses run on power we made ourselves." />
      <Stat big="4 sites" text="across Calderdale and Kirklees: two wind farms, a solar farm, and a battery store." />
      <Stat big="2 min" text="average wait to reach a person on the phone, every day of the week." />
    </section>
  );
}

function MeetWren() {
  return (
    <section id="wren" style={{ background: "var(--moor-tint)" }}>
      <div style={{ ...wrap, padding: "88px 32px", display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,380px),1fr))", gap: 56, alignItems: "center" }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 20 }}>
          <span className="mono" style={{ fontSize: 12, color: "oklch(0.35 0.06 165)" }}>Meet Wren</span>
          <h2 className="h2">An assistant that explains your bill, then asks before it changes anything.</h2>
          <p style={{ margin: 0, fontSize: 17, lineHeight: 1.6, maxWidth: "48ch", color: "oklch(0.32 0.03 165)" }}>
            Wren can read your meter history, explain a bill line by line and find you a cheaper tariff. It always tells you what it checked, and it never switches, pays or cancels without your OK.
          </p>
          <a className="btn btn-primary" href={PORTAL} style={{ alignSelf: "flex-start" }}>Try Wren in your account</a>
        </div>
        <div style={{ background: "var(--card)", borderRadius: "var(--r-card-lg)", padding: 28, display: "flex", flexDirection: "column", gap: 18, boxShadow: "var(--float)" }}>
          <UserBubble>Why is my October bill higher?</UserBubble>
          <div style={{ display: "flex", gap: 10, alignItems: "flex-start" }}>
            <AgentAvatar />
            <div style={{ display: "flex", flexDirection: "column", gap: 10 }}>
              <p className="agent-text" style={{ lineHeight: 1.5 }}>It's up £14. You used 38 kWh more, mostly between the 20th and 24th, when it was 3°C overnight. Your unit rate didn't change.</p>
              <AgentReceipt items={[
                { tool: "read_bill", detail: "October" },
                { tool: "read_usage", detail: "half-hourly" },
                { tool: "get_weather", detail: "HX7" },
              ]} />
            </div>
          </div>
        </div>
      </div>
    </section>
  );
}

function Tariff({ t, tone, copy }) {
  return (
    <div className="card card-lg" style={{ padding: 32, display: "flex", flexDirection: "column", gap: 20 }}>
      <div style={{ display: "flex", justifyContent: "space-between", gap: 12, alignItems: "center" }}>
        <span style={{ fontSize: 20, fontWeight: 600 }}>{t.name}</span>
        <Badge tone={tone}>{t.badge}</Badge>
      </div>
      <p className="body-copy" style={{ margin: 0, fontSize: 16, lineHeight: 1.55 }}>{copy}</p>
      <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
        <span className="big" style={{ fontSize: 48 }}>{t.rate}</span>
        <span className="muted" style={{ fontSize: 14 }}>per kWh · {TARIFFS.standing}</span>
      </div>
    </div>
  );
}

function Tariffs() {
  return (
    <section id="tariffs" style={{ ...wrap, padding: "88px 32px", display: "flex", flexDirection: "column", gap: 40 }}>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "flex-end", gap: 24, flexWrap: "wrap" }}>
        <h2 className="h2" style={{ maxWidth: "18ch" }}>Two tariffs. Both 100% our own power.</h2>
        <a className="btn-link" href="#tariffs" style={{ fontSize: 15, fontWeight: 500 }}>Business tariffs</a>
      </div>
      <div style={{ display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,340px),1fr))", gap: 24 }}>
        <Tariff t={TARIFFS.steady} tone="neutral" copy="One price for every hour of the day. Good if you want to stop thinking about it." />
        <Tariff t={TARIFFS.flex} tone="wind" copy="Your price drops when our turbines are making more than we need. We'll text you the best hours the night before." />
      </div>
    </section>
  );
}

function Sites() {
  return (
    <section id="sites" style={{ ...wrap, padding: "0 32px 88px", display: "grid", gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,280px),1fr))", gap: 24 }}>
      {SITES.map((s) => (
        <div key={s.name} style={{ display: "flex", flexDirection: "column", gap: 14 }}>
          <div role="img" aria-label={s.name} style={{ height: 220, borderRadius: "var(--r-card-lg)", background: "var(--photo-bg) url('" + PHOTOS + s.photo + "') center/cover no-repeat" }} />
          <span style={{ fontSize: 17, fontWeight: 600 }}>{s.name}</span>
          <span className="muted" style={{ fontSize: 14 }}>{s.detail}</span>
        </div>
      ))}
    </section>
  );
}

function Footer() {
  const col = { display: "flex", flexDirection: "column", gap: 10 };
  const head = { color: "var(--footer-head)", fontWeight: 600 };
  const link = { color: "inherit" };
  return (
    <footer style={{ background: "var(--peat)", color: "var(--footer-ink)" }}>
      <div style={{ ...wrap, padding: "56px 32px", display: "flex", justifyContent: "space-between", gap: 40, flexWrap: "wrap" }}>
        <div style={{ display: "flex", flexDirection: "column", gap: 12, maxWidth: 320 }}>
          <span className="serif" style={{ fontSize: 28, color: "var(--footer-head)" }}>Wrenmoor Energy</span>
          <span style={{ fontSize: 14, lineHeight: 1.55 }}>Made on the moor in West Yorkshire. Wrenmoor Energy Ltd, Hebden Bridge.</span>
        </div>
        <div style={{ display: "flex", gap: 56, flexWrap: "wrap", fontSize: 14 }}>
          <div style={col}><span style={head}>Customers</span><a href={PORTAL} style={link}>Your account</a><a href={PORTAL} style={link}>Submit a reading</a><a href="#" style={link}>Report a power cut</a></div>
          <div style={col}><span style={head}>Company</span><a href="#sites" style={link}>About us</a><a href="#" style={link}>Careers</a><a href={INTRANET} style={link}>Staff login</a></div>
        </div>
      </div>
    </footer>
  );
}

function App() {
  return (
    <div style={{ display: "flex", flexDirection: "column" }}>
      <Header />
      <main>
        <Hero />
        <Stats />
        <MeetWren />
        <Tariffs />
        <Sites />
      </main>
      <Footer />
      <Disclaimer />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
