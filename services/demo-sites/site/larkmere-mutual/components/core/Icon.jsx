import React from "react";

const pascal = (n) => String(n).split(/[-_ ]/).map(s => s.charAt(0).toUpperCase() + s.slice(1)).join("");

export function Icon({ name, size = 20, strokeWidth = 1.5, style, title }) {
  const ref = React.useRef(null);
  React.useEffect(() => {
    const host = ref.current;
    const L = typeof window !== "undefined" ? window.lucide : null;
    if (!host) return;
    host.innerHTML = "";
    if (!L) return;
    const node = L.icons && (L.icons[pascal(name)] || L.icons[name]);
    if (node && L.createElement) {
      const svg = L.createElement(node);
      svg.setAttribute("width", size);
      svg.setAttribute("height", size);
      svg.setAttribute("stroke-width", strokeWidth);
      svg.style.display = "block";
      host.appendChild(svg);
    } else {
      host.innerHTML = '<i data-lucide="' + name + '"></i>';
      L.createIcons && L.createIcons({ attrs: { width: size, height: size, "stroke-width": strokeWidth } });
    }
  }, [name, size, strokeWidth]);
  return (
    <span
      ref={ref}
      aria-hidden={title ? undefined : true}
      role={title ? "img" : undefined}
      aria-label={title}
      style={{ display: "inline-flex", width: size, height: size, flex: "0 0 auto", color: "currentColor", ...style }}
    />
  );
}
