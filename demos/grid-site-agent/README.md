# Wrenmoor Edge on-site agent (dial-home)

Wrenmoor Energy runs a grid-operations assistant **inside the operational
network** of its Wrenmoor Edge wind farm (29 MW) and 33 kV collector
substation. That network accepts no inbound connections, and the assistant's
tools have to run there, next to the plant: it reads live values from the
site SCADA and raises maintenance work orders.

The agent opens one outbound connection to Omnia, the
[dial-home channel](https://github.com/AltairaLabs/omnia-dialhome), and Omnia
treats it as one of its own agents: the console can talk to it, and its
conversations are recorded as sessions, which evals run on.

```
 Wrenmoor Edge operational network          |  Omnia (wrenmoor-energy namespace)
 (no inbound connections)                   |
                                            |
  grid-site-agent  --- one outbound wss --->|-->  relay  ---  facade  <---  console
   PromptKit pack                           |     (agent pod: grid-site-agent)
   list_assets, read_scada,                 |                    |
   raise_work_order run HERE                |                    v
                                            |              session-api, evals
```

## What is in it

| Path | What it is |
|---|---|
| `arena/` | The pack's source: the prompt and the three tools. Compiled by `hack/build-packs.sh grid-site-agent` into `charts/omnia-demo/files/arena/grid-site-agent-pack.json` |
| `main.go` | The on-site entrypoint: dials home with the key Omnia issued and serves the pack over the channel, using the [omnia-dialhome](https://github.com/AltairaLabs/omnia-dialhome) library |
| `site.go` | The site's SCADA and maintenance system: fictional, deterministic, in memory. The tools run here, in this process |
| `script.go` | The scripted model: PromptKit's mock provider choosing the next move from the engineer's message, so no model and no key are needed |
| `site/kubernetes.yaml` | The site as a namespace: NetworkPolicy allowing no ingress and egress only to the relay |
| `site/compose.yaml` | The site as one container with no published ports |
| `Dockerfile` | `ghcr.io/altairalabs/omnia-demo-grid-site-agent`, tagged `latest` and `sha-<sha>` |

The Omnia side is the chart's `gridSite` values: the `grid-site-agent`
AgentRuntime with the `omnia.altairalabs.ai/dial-home` annotation (its pod
runs Omnia's relay, `omnia-remote-runtime`, in place of a runtime), its
PromptPack, and the `grid-site-policy` ToolPolicy.

### How the site's tools run

The tools are **local tool handlers**, not client tools. `omnia-dialhome`'s
`promptkit` handler runs each pack tool that has an `OnTool` handler in the
site process, and nothing about the call crosses the channel; a client tool
would instead be sent to whoever is talking to the agent (the console), which
is the wrong place for a SCADA read. The released `omnia-dialhome` command can
only run client tools, so the site has its own small `main.go` on the
library ([omnia-dialhome#5](https://github.com/AltairaLabs/omnia-dialhome/issues/5)).

## What this demo does not show yet

Say these plainly if asked; they are open work, not features.

- **Omnia does not yet decide the site's tool calls.** The `grid-site-policy`
  ToolPolicy refuses a work order above £25,000, but nothing on the dial-home
  path asks Omnia before a site tool runs, so today the £48,000 transformer
  order below **is raised**.
  [omnia-dialhome#2](https://github.com/AltairaLabs/omnia-dialhome/issues/2)
  tracks carrying the decision over the channel. The test
  `TestAnOrderAboveTheLimitIsRaisedUntilPolicyIsConsulted` pins today's
  behaviour, so the change shows up when it lands.
- **The site's tool calls are not on the session.** The engineer's message
  and the reply are recorded; the tool call between them is not
  ([omnia-dialhome#3](https://github.com/AltairaLabs/omnia-dialhome/issues/3)).
- **No memory.** The channel carries the memory stream, but nothing gives the
  pack Omnia memory over it yet
  ([omnia-dialhome#4](https://github.com/AltairaLabs/omnia-dialhome/issues/4)).
- **The relay is told the site is on Vertex.** Omnia's relay accepts relay mode
  only with `OMNIA_REMOTE_PLATFORM=vertex`, and an on-site runtime has no
  platform of its own to name. Nothing in this demo depends on the value.

## Run it

### Prerequisites

- An Omnia install with Enterprise enabled (dev mode is fine): the relay and
  ToolPolicy are Enterprise. Its operator must support the dial-home
  annotation.
- A Workspace for Wrenmoor Energy, e.g. the `wrenmoor-energy` tenant from
  `profiles/`. The commands below assume namespace `wrenmoor-energy` and kube
  context `<ctx>`.

### 1. Omnia's side

```bash
helm --kube-context <ctx> upgrade --install wrenmoor-energy charts/omnia-demo \
  -n wrenmoor-energy --reuse-values --set gridSite.enabled=true
```

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
agent out only to the relay's port. Inside a cluster the in-cluster
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
| `Raise an urgent work order to regenerate the oil and replace two cooling fans on the transformer, estimated £48,000` | `raise_work_order` | `Raised WO-WE-0413 …`: **today**. Once the policy decision crosses the channel, refused by `grid-site-policy` |
| `Open breaker WE-CB3` | nothing | It does not operate plant |

Work-order numbers restart at WO-WE-0412 when the site process restarts.

Then show the record: the session is in Omnia with each message and reply,
although the conversation ran on the site.

### 5. Revoke it

```bash
kubectl --context <ctx> -n wrenmoor-energy delete secret grid-site-agent-dial-home-key
```

The relay closes the site's connection and the agent goes NotReady. The site
keeps re-dialling and is refused with 401 until it is given the new key.

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
That spends real model money; the scripted model spends none.

## Develop

```bash
go test ./demos/grid-site-agent/                 # tools, script, and a turn over a stand-in relay
hack/build-packs.sh grid-site-agent              # after editing arena/
docker build -f demos/grid-site-agent/Dockerfile -t omnia-demo-grid-site-agent:dev .
```

All data here is fictional.
