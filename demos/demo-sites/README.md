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
site/shared/            vendored React, ReactDOM, Babel, lucide; kit-loader.js, react-shim.js
site/larkmere-mutual/   tokens, components, assets, ui_kits/{marketing,portal}
site/wrenmoor-energy/   tokens, components, assets, ui_kits/{marketing,portal,intranet}
```

A brand's directory is served over `shared/`, so `<site>/vendor/...` and
`<site>/kit-loader.js` resolve for every brand. The pages are React rendered by
in-browser Babel from those vendored scripts: the cluster has no egress, so
nothing is hot-linked. (Larkmere Mutual's fonts still come from Google Fonts,
as before; Wrenmoor Energy's are vendored woff2 files.)

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
