# Wrenmoor Edge on-site agent (dial-home)

Wrenmoor Energy runs a grid-operations assistant **inside the operational
network** of its Wrenmoor Edge wind farm (29 MW) and 33 kV collector
substation. That network accepts no inbound connections, and the assistant's
tools have to run there, next to the plant: it reads live values from the
site SCADA and raises maintenance work orders.

The agent opens one outbound connection to Omnia, the
[dial-home channel](https://github.com/AltairaLabs/omnia-dialhome), and Omnia
treats it as one of its own agents: the console can talk to it, its
conversations are recorded as sessions, and **every tool it runs on site is
put to Omnia's tool policy first**. The site asks over the channel before each
tool runs, and runs it only if Omnia allows it. Each call it made is on the
session, and a call Omnia refused is recorded there as denied. (The pack
declares no evals; any workspace eval that reads sessions sees these like any
other agent's.)

```
 Wrenmoor Edge operational network          |  Omnia (wrenmoor-energy namespace)
 (no inbound connections)                   |
                                            |
  grid-site-agent  -- one outbound ws(s) -->|-->  relay  ---  facade  <---  console
   PromptKit pack                           |     (agent pod: grid-site-agent)
   list_assets, read_scada,                 |       |         |
   raise_work_order run HERE,               |       v         v
   each one only after Omnia allows it      |  policy broker  session-api
                                            |  (grid-site-policy)
```

## What is in it

| Path | What it is |
|---|---|
| `arena/` | The pack's source: the prompt and the three tools. Compiled by `hack/build-packs.sh grid-site-agent` into `charts/omnia-demo/files/arena/grid-site-agent-pack.json` |
| `main.go` | The on-site entrypoint: dials home with the key Omnia issued and serves the pack over the channel, using the [omnia-dialhome](https://github.com/AltairaLabs/omnia-dialhome) library, with every site tool put to Omnia's tool policy and recorded on the session |
| `site.go` | The site's SCADA and maintenance system: fictional, deterministic, in memory. The tools run here, in this process |
| `script.go` | The scripted model: PromptKit's mock provider choosing the next move from the engineer's message, so no model and no key are needed |
| `site/kubernetes.yaml` | The site as a namespace: NetworkPolicy allowing no ingress and egress only to the relay |
| `site/compose.yaml` | The site as one container with no published ports |
| `Dockerfile` | `ghcr.io/altairalabs/omnia-demo-grid-site-agent`, tagged `latest` and `sha-<sha>` |

The Omnia side is the chart's `gridSite` values: the `grid-site-agent`
AgentRuntime with the `omnia.altairalabs.ai/dial-home` annotation (its pod
runs Omnia's relay, `omnia-remote-runtime`, in place of a runtime, with
`OMNIA_REMOTE_PLATFORM=self-hosted`), its PromptPack, and the
`grid-site-policy` ToolPolicy, which refuses a work order above £25,000
(`gridSite.workOrderLimitGBP`).

### How the site's tools run

The tools are **local tool handlers**, not client tools. `omnia-dialhome`'s
`promptkit` handler runs each pack tool that has an `OnTool` handler in the
site process; a client tool would instead be sent to whoever is talking to
the agent (the console), which is the wrong place for a SCADA read. The
released `omnia-dialhome` command can only run client tools, so the site has
its own small `main.go` on the library
([omnia-dialhome#5](https://github.com/AltairaLabs/omnia-dialhome/issues/5)).

### How Omnia governs them

The tool runs on site, but the decision is Omnia's:

1. Each site tool is declared in Omnia, in the `grid-site-tools` ToolRegistry,
   as a `remote` handler: its name, description and schema, and no endpoint,
   since only the site runs it. Omnia refuses a tool the site runs that no such
   handler declares.
2. Before a tool runs, the site sends Omnia the call (tool and arguments) over
   the channel, naming only the conversation.
3. Omnia's relay asks the policy broker in the agent's pod, as Omnia's own
   runtime does, under the registry that declares the tool, for the engineer
   the console sent with that conversation.
   `grid-site-policy` permits the site's tools and forbids a work order whose
   `estimated_cost_gbp` is above the limit.
4. On an allow the tool runs, and the site records the call on the session.
   On a refusal it does not run. The model is told why in the policy's
   `@message`, and Omnia records the call on the session as **denied**.
5. If no decision comes (the channel is down, say), the tool does not run
   either. Nothing on site runs without Omnia's say-so.

## What this demo does not show

Say these plainly if asked.

- **Memory.** The site can use the memory Omnia keeps for the engineer
  (`SITE_AGENT_MEMORY=true`, with `spec.memory` enabled on the AgentRuntime),
  but this demo's chart does not enable it and the scripted model never
  calls a memory tool.
- **Approval.** A policy that refers a call for a person's approval is
  refused by Omnia's relay rather than held: a site cannot hold a call while a
  person decides.
- **A dishonest site.** The site asks before every tool. A site built to skip
  the question would run its tools ungoverned, as any code holding the agent's
  key could. What the demo shows is that an honest site is governed exactly
  as an in-cluster agent is, and fails closed when it cannot be.

## Run it

### Prerequisites

- An Omnia install with Enterprise enabled (dev mode is fine): the relay and
  ToolPolicy are Enterprise. Its operator must support the dial-home
  annotation (it injects the policy broker beside the relay, as for any
  Enterprise agent), and its relay image must accept
  `OMNIA_REMOTE_PLATFORM=self-hosted` and serve the dial-home policy stream:
  an older relay refuses the platform at start-up.
- A Workspace for Wrenmoor Energy, e.g. the `wrenmoor-energy` tenant from
  `profiles/`. The commands below assume namespace `wrenmoor-energy` and kube
  context `<ctx>`.

### 1. Omnia's side

```bash
helm --kube-context <ctx> upgrade --install wrenmoor-energy charts/omnia-demo \
  -n wrenmoor-energy --reset-then-reuse-values --set gridSite.enabled=true
```

`--reset-then-reuse-values` (Helm 3.14+) takes this chart's defaults for the
new `gridSite` block and keeps the release's own values; plain
`--reuse-values` would drop the new defaults. Passing the tenant's profile
files again with `-f` works too.

The operator issues the site's key into Secret
`grid-site-agent-dial-home-key` and exposes the relay as Service
`grid-site-agent-dial-home`, port 9002. The agent pod stays NotReady until
the site connects; that is expected.

```bash
kubectl --context <ctx> -n wrenmoor-energy get secret grid-site-agent-dial-home-key
```

`gridSite.relay.image.tag` defaults to `main`, Omnia's own tag for that
image; pin it to the Omnia build you run.

### 2a. The site as a namespace (same cluster)

The namespace plays the site: its NetworkPolicy lets nothing in, and lets the
agent out only to the relay's port. If the Workspace isolates its namespace
(`networkPolicy.isolate: true`), also allow ingress from
`wrenmoor-edge-site` to port 9002 there. Inside a cluster the in-cluster
`ws://…svc.cluster.local` address is allowed; anywhere else the URL must be
`wss://`.

```bash
kubectl --context <ctx> apply -f demos/grid-site-agent/site/kubernetes.yaml
kubectl --context <ctx> -n wrenmoor-energy get secret grid-site-agent-dial-home-key \
  -o jsonpath='{.data.key}' | base64 -d > /tmp/grid-site.key
kubectl --context <ctx> -n wrenmoor-edge-site create secret generic grid-site-dial-home \
  --from-file=key=/tmp/grid-site.key \
  --from-literal=url=ws://grid-site-agent-dial-home.wrenmoor-energy.svc.cluster.local:9002/v1/channel
rm /tmp/grid-site.key
kubectl --context <ctx> -n wrenmoor-edge-site rollout restart deployment/grid-site-agent
```

### 2b. The site as a container (another machine)

Expose the relay over TLS first: with Omnia's default-exposure Gateway the
Secret's `url` key holds the `wss://` address; otherwise route
`grid-site-agent-dial-home:9002` through your own TLS ingress.

```bash
export OMNIA_DIAL_HOME_URL=$(kubectl --context <ctx> -n wrenmoor-energy get secret \
  grid-site-agent-dial-home-key -o jsonpath='{.data.url}' | base64 -d)
export OMNIA_DIAL_HOME_KEY=$(kubectl --context <ctx> -n wrenmoor-energy get secret \
  grid-site-agent-dial-home-key -o jsonpath='{.data.key}' | base64 -d)
docker compose -f demos/grid-site-agent/site/compose.yaml up
```

The container publishes no ports. On Linux without a Gateway,
`site/compose.yaml` shows the loopback port-forward variant.

### 3. Check it connected

```bash
kubectl --context <ctx> -n wrenmoor-energy get pods -l app.kubernetes.io/instance=grid-site-agent
kubectl --context <ctx> -n wrenmoor-edge-site get pods   # READY 1/1 once the channel is up
```

The agent pod turns Ready once the site holds the channel.

### 4. Talk to it

Open `grid-site-agent` in the Omnia console and send these lines. The model is
scripted (`script.go`); the tools, the channel and the session record are
live. "The model's turns are scripted for the recording; the tools run on the
site" is the honest answer if asked.

| Say | The site does | Reply |
|---|---|---|
| `How is the site? Any alarms?` | `list_assets` | 5 assets, 2 alarms: WE-GT1 top-oil temperature, WE-T07 pitch fault |
| `Read turbine 7` | `read_scada` WE-T07 | Stopped, pitch system fault on blade B, 0 kW at 11.2 m/s |
| `SCADA for WE-GT1` | `read_scada` WE-GT1 | Top oil 71 °C, hot spot 84 °C, 2 of 4 cooling fans |
| `Raise an urgent work order to replace the blade B pitch motor on turbine 7, estimated £6,400` | `raise_work_order` | `Raised WO-WE-0412 …` |
| `Raise an urgent work order to regenerate the oil and replace two cooling fans on the transformer, estimated £48,000` | nothing: Omnia refuses `raise_work_order` before it runs | `Omnia refused that. This work order is above the £25000 the site assistant may commit without a human …` |
| `Open breaker WE-CB3` | nothing | It does not operate plant |

Work-order numbers restart at WO-WE-0412 when the site process restarts.

Then show the record: the session is in Omnia with each message and reply,
although the conversation ran on the site. Its tool calls show each site tool
that ran, with its arguments and what the site returned, and the £48,000 order
as **denied** by `deny-work-order-above-site-limit`, with the policy's message.
The policy broker's decisions (its logs and
`omnia_toolpolicy_decisions_total`) show the same refusal.

### 5. Revoke it

```bash
kubectl --context <ctx> -n wrenmoor-energy delete secret grid-site-agent-dial-home-key
```

The relay closes the site's connection and the agent goes NotReady. The site
keeps re-dialling and is refused with 401 until it is given the new key: the
operator issues one into the same Secret, so re-run step 2a's Secret and
restart (or re-export the key for 2b).

## Clean up

```bash
kubectl --context <ctx> delete namespace wrenmoor-edge-site
helm --kube-context <ctx> upgrade wrenmoor-energy charts/omnia-demo -n wrenmoor-energy \
  --reuse-values --set gridSite.enabled=false
```

## Run on a real model

`SITE_AGENT_MODEL=omnia` builds the model from the Provider the agent lists,
served over the channel: set `gridSite.providerRef` to a Provider whose
credential the site can resolve itself (the relay withholds one keyed by a
cluster Secret; give the site that configuration as `OMNIA_DIAL_HOME_CONFIG`).

Only the Provider's *configuration* comes over the channel. The model calls
go from the site straight to the provider's endpoint, so the site must be
allowed out to it: in the namespace variant, add an egress rule for it to
`site/kubernetes.yaml`, whose `dial-home-only` policy otherwise refuses every
model call. That spends real model money; the scripted model spends none (it
reports no cost).

## Develop

```bash
go test ./demos/grid-site-agent/                 # tools, script, the policy refusal, and turns over a stand-in relay
hack/build-packs.sh grid-site-agent              # after editing arena/
docker build -f demos/grid-site-agent/Dockerfile -t omnia-demo-grid-site-agent:dev .
```

All data here is fictional.
