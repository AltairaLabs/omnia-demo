# Omnia public memory demo

This chart is an add-on for an existing Omnia installation. It deploys a
local MinIO source, uploads the synthetic fixtures, and runs the public example
ingestion service once. It does not install or copy Omnia's private templates.

Install Omnia separately, with Enterprise institutional ingestion enabled, then
install this chart:

```bash
helm upgrade --install omnia-demo ./charts/omnia-demo \
  --namespace omnia-demo --create-namespace \
  --set memory.apiURL=http://<memory-api-service>.<omnia-namespace>:8080
```

The chart is intentionally an add-on rather than an umbrella chart. This keeps
the public repository independent of private Omnia implementation details and
makes the dependency on a released Omnia version explicit in the operator's
installation procedure.
