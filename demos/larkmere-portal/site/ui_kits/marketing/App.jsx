import React, { useState, useEffect } from "react";
import { SiteHeader, SiteFooter } from "./Chrome.jsx";
import { Home } from "./Home.jsx";
import { Protection } from "./Protection.jsx";
import { Banking } from "./Banking.jsx";
import { About } from "./About.jsx";

const SCREENS = { home: Home, protection: Protection, banking: Banking, about: About };

function App() {
  const [route, setRoute] = useState("home");
  const Screen = SCREENS[route] || Home;
  useEffect(() => {
    const t = setTimeout(() => window.lucide && window.lucide.createIcons(), 40);
    return () => clearTimeout(t);
  }, [route]);
  return (
    <div>
      <SiteHeader route={route} onNavigate={setRoute} />
      <Screen onNavigate={setRoute} />
      <SiteFooter onNavigate={setRoute} />
    </div>
  );
}

ReactDOM.createRoot(document.getElementById("root")).render(<App />);
