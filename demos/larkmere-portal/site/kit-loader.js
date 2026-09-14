/* Larkmere UI-kit loader.
   Fetches ES modules (.jsx / .js), transforms JSX with Babel standalone, rewrites
   relative + "react" specifiers to blob URLs, and imports the graph. Lets the kits
   import the real component sources in components/ with no build step. */
(function () {
  const cache = new Map();

  async function load(url, shimUrl) {
    if (cache.has(url)) return cache.get(url);
    const p = (async () => {
      const res = await fetch(url);
      if (!res.ok) throw new Error("Could not load " + url + " (" + res.status + ")");
      const src = await res.text();
      let code = window.Babel.transform(src, {
        presets: [["react", { runtime: "classic" }]],
        sourceType: "module",
        filename: url.split("/").pop(),
      }).code;

      const specs = new Set();
      const re = /(?:^|[\s;})])(?:from|import)\s*["']([^"']+)["']/g;
      let m;
      while ((m = re.exec(code))) {
        const s = m[1];
        if (/[\s,]/.test(s)) continue;
        if (s === "react" || /^\.{1,2}\//.test(s)) specs.add(s);
      }

      for (const spec of specs) {
        const target = spec === "react" ? shimUrl : new URL(spec, url).href;
        const blob = await load(target, shimUrl);
        code = code.split('"' + spec + '"').join('"' + blob + '"').split("'" + spec + "'").join('"' + blob + '"');
      }
      return URL.createObjectURL(new Blob([code], { type: "text/javascript" }));
    })();
    cache.set(url, p);
    return p;
  }

  async function mount(entry, shim) {
    const shimUrl = new URL(shim || "../../react-shim.js", document.baseURI).href;
    try {
      const blob = await load(new URL(entry, document.baseURI).href, shimUrl);
      await import(blob);
      if (window.lucide) window.lucide.createIcons();
    } catch (err) {
      console.error(err);
      document.body.insertAdjacentHTML(
        "beforeend",
        '<pre style="font:13px/1.5 ui-monospace,monospace;color:#8C2F27;padding:20px;white-space:pre-wrap">' +
          String(err && err.stack || err) + "</pre>"
      );
    }
  }

  window.KitLoader = { mount, load };
})();
