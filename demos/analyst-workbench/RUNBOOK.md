# Analyst workbench (Wrenmoor Energy)

An analyst asks a question that needs more than a lookup, and the agent
answers it by writing Python, running it in a sandbox and reading the result.
The point to make: the sandbox is the control, not the prompt. Each sandbox
is its own Kata VM on omnia-azure's `sandbox` pool, with no service-account
token, no DNS and no network except the generation data service.

Enabled in `profiles/azure/wrenmoor-energy.yaml` since Omnia `9f0b955`, which
includes #4147 (Omnia #4112). **Not yet run live on that build**: do the
checks below once before the first take, then mark it Live in the catalogue.

## Why it was off

The namespace is in Istio's ambient mesh with a waypoint. Before #4147:

- a Kata sandbox was unreachable, because ambient delivers inbound mesh
  traffic to ztunnel in the pod's network namespace and the VM has taken it;
- on runc, the sandbox's one allowed egress (wrenmoor-grid-data) went to the
  waypoint on :15008, which the sandbox's NetworkPolicy drops.

Omnia now labels every sandbox pod, its egress proxy and its enrolment pod
`istio.io/dataplane-mode: none`. The mesh never captures them, so the runtime
reaches the VM and the sandbox reaches grid-data in plaintext on the port the
NetworkPolicy allows. Nothing in this repo opts the sandbox out: the operator
does it for every sandbox.

The limit Omnia documents: a pod outside the mesh has no mesh identity, so a
destination under a STRICT PeerAuthentication refuses it. wrenmoor-grid-data
has none (omnia-azure sets no PeerAuthentication), so plaintext is accepted.
If that changes, add a PeerAuthentication selecting only grid-data's pods with
`mtls.mode: PERMISSIVE`; the sandbox's NetworkPolicy still decides who calls.

## Checks (once, after the upgrade)

The sandbox image, `omnia-demo-analyst-sandbox`, must exist where the chart
looks. `make images` builds it into the dev registry with the others; for
ghcr, run the publish workflow and pin its `sha-<short>` as
`analystWorkbench.image.tag` in the Wrenmoor profile. Otherwise the sandbox
pod sits in ImagePullBackOff and the first tool call times out after 8m.

Dashboard, **Console**, `analyst-workbench`. Ask:

> "What was Wrenmoor Edge's capacity factor each day last week? Chart it."

The first call waits for a sandbox node (the pool scales from zero, a few
minutes; the handler's timeout is 8m), the image pull and the VM boot. Then:

```bash
# The sandbox pod: on a Kata node, out of the mesh.
kubectl --context aks-omnia-demo -n wrenmoor-energy get pods \
  -l app.kubernetes.io/component=sandbox,app.kubernetes.io/part-of=analyst-workbench \
  -o wide --show-labels
kubectl --context aks-omnia-demo get runtimeclass kata-vm-isolation
# The class: Ready, the runtime family it resolved, NetworkPolicy proven.
kubectl --context aks-omnia-demo -n wrenmoor-energy get sandboxclass analyst-sandbox -o yaml
```

Check:

- the pod carries `istio.io/dataplane-mode=none` and runs on a `sandbox` node;
- the answer has figures from the data service, so egress to grid-data works;
- the sandbox reaches nothing else: ask it to fetch `https://example.com` with
  Python and to resolve a host name; both fail.

## On stage

1. The question above. It writes `analysis.py`, runs it and answers with the
   capacity factors and a chart file.
2. "Where does that code run?" Show the sandbox pod and its node: its own VM.
3. "What can it reach?" Ask it to fetch a public URL. It can't: the sandbox's
   NetworkPolicy allows the data service and nothing else, and no prompt line
   says so.

Say "its own VM" only when the pod is on the Kata pool. With an empty
`runtimeClassName` the sandbox runs on runc, with no VM boundary.
