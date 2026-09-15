# Mortgage underwriting

This example is a synthetic backend underwriting workflow. The Go service
provides case, credit, affordability, AML, valuation, income-verification, and
case-note endpoints; the Arena project defines the agent prompt and tools.

Build and run the service locally:

```bash
go run ./demos/mortgage-underwriting
```

The service contains fictional data only. The Arena source is compiled and
deployed by an Omnia Enterprise installation; this repository does not include
private platform manifests or credentials.
