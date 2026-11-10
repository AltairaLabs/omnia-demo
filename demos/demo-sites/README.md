# Demo sites

The demo companies' customer-facing websites, in one small Go server and one
image (`ghcr.io/altairalabs/omnia-demo-sites`). `--site` picks the brand:

| `--site` | What it serves | Agent behind the chat panel |
|---|---|---|
| `larkmere-mutual` | Larkmere Mutual's public site and member portal | `piiChat` (card support) |
| `wrenmoor-energy` | Wrenmoor Energy's public site, customer portal and staff intranet ("Wrenmoor Hub") | `wren` |

Every site is fictional and carries synthetic content. Do not point one at a
real customer system without adding authentication and security controls.

## Layout

```
site/shared/            vendored React, ReactDOM, lucide; react-shim.js
site/larkmere-mutual/   tokens, components, assets, ui_kits/{marketing,portal}
site/wrenmoor-energy/   tokens, components, assets, ui_kits/{marketing,portal,intranet}
site/halvern-partners/  tokens, assets, ui_kits/{marketing,practice-desk,design-system}
```

A brand's directory is served over `shared/`, so `<site>/vendor/...` resolves
for every brand. The pages are React: `hack/build-kits.sh` compiles each UI
kit's `App.jsx` and its imports into one `app.js` beside its `index.html`
(esbuild; `react` resolves to `shared/react-shim.js`, which hands over the
vendored UMD global). The Dockerfile runs it before `go build` embeds the
site; run it yourself before a local `go run`. The bundles are gitignored.

The design exports loaded their JSX in the browser through Babel standalone
instead: a 2.4 MB download and one round trip per module, which behind the
catalogue's sign-in proxy kept Larkmere Mutual's page white for 15 seconds.

Text files are served gzipped, each compressed once and cached; the vendored
scripts, fonts and photos carry a day's Cache-Control. The cluster has no
egress, so nothing is hot-linked. (Larkmere Mutual's fonts still come from
Google Fonts, as before; Wrenmoor Energy's and Halvern Partners' are vendored
woff2 files.)

Halvern Partners' pages are plain HTML with a little script, converted from
the design handoff's prototypes; they use none of the shared React scripts.

## Routes

| Path | |
|---|---|
| `/` | relative redirect to `ui_kits/marketing/` |
| `/healthz` | readiness |
| `/api/assistant/ws` | WebSocket proxy to `--agent-url` `/ws`, asserting the customer's identity (`x-user-id`, `x-user-email`, `x-user-roles`) for the AgentRuntime's `externalAuth.edgeTrust` |
| `/api/assistant/consent` | POST, proxied to `--backend-url` `/wren/consent` (Wrenmoor Energy only) |
| `/api/assistant/receipts` | GET, proxied to `--backend-url` `/wren/receipts` (Wrenmoor Energy only) |

The Wrenmoor Energy pages address all of these relative to the page, so the
site works under a path prefix with no rewrites. The Larkmere Mutual portal
opens its socket at the absolute `/api/assistant/ws`.

## Run locally

```bash
go run ./demos/demo-sites --site wrenmoor-energy --addr :8091 \
  --agent-url http://localhost:8080 --backend-url http://localhost:8081
go run ./demos/demo-sites --site larkmere-mutual --addr :8092
```
