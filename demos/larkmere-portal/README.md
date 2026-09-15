# Larkmere member portal

This is the fictional member-facing frontend and proxy used by the PII chat
example. It embeds the static UI in a small Go server and forwards the browser
chat connection to the configured agent facade.

Build and run it locally:

```bash
go run ./demos/larkmere-portal
```

Set the agent URL and any required proxy configuration through environment
variables. The portal contains synthetic content and must not be pointed at a
real member system without adding authentication and security controls.

The public image is published as
`ghcr.io/altairalabs/omnia-demo-larkmere-portal:latest`.
