# Install the demo tenants into an Omnia install.
#
# One Workspace per company in the fictional cast, each an omnia-demo release
# in its own namespace. The platform-specific values live in profiles/<PROFILE>/
# (common.yaml under one file per tenant). The azure profile installs onto your
# Omnia on AKS, described by deploy/azure/site.yaml (copy site.example.yaml): per
# tenant it makes the workspace's Azure side (deploy/azure/tenant.sh), installs
# the chart with the profile and your site's values (deploy/azure/site-values.sh),
# then federates the agents (deploy/azure/identity.sh).
#
#   make demos                          every tenant
#   make demos TENANTS=wrenmoor-energy  one tenant
#   make builder                        the Arena builder, for Halvern Partners
#   make images                         build every image into the dev registry
#
# `make images` builds each demo image into your registry (site.yaml images.*)
# with `az acr build`, tagged dev-<commit>, and records the registry and tag in
# .images.env. While that file exists, `make demos` runs
# every image from that build instead of the azure profile's pins; delete it to
# go back. Nothing is published from this repository.
#
# Requires deploy/azure/site.yaml, mikefarah yq v4, and az/kubectl/helm access to
# your cluster. SITE points at another site file (make lint SITE=...).

SHELL := bash

PROFILE          ?= azure
# ellens-assistant is not a company: it is the person's side of the Poppy demo
# (profiles/azure/ellens-assistant.yaml). Its first install needs, in order:
# `make azure-tenant WS=ellens-assistant` (the namespace),
# `CONTEXT=$(KUBE_CONTEXT) hack/personal-agent-keys.sh` (its keys) and
# `make images` (no personal-agent image is pinned yet); without the keys or
# the image, `make demos` waits out its 10m timeout.
# ellens-assistant before halvern-partners: the catalogue's nginx (in halvern-partners) resolves assistant-page.ellens-assistant at start and crashes if that Service doesn't exist yet.
TENANTS          ?= larkmere-mutual wrenmoor-energy ellens-assistant halvern-partners halvern-research
# Your install's description; the kubectl/helm context comes from it too.
SITE             ?= deploy/azure/site.yaml
export SITE
CHART            ?= charts/omnia-demo
BUILDER_TENANT   ?= halvern-partners
BUILDER_PROVIDER ?= azure-foundry-gpt-5-4-mini

# The canary, research and build-loop targets name their cluster by hand:
# KUBE_CONTEXT is aks.kubeContext in your site.yaml.
KUBE_CONTEXT     ?= $(or $(shell yq e '.aks.kubeContext | select(. != null)' $(SITE) 2>/dev/null),$(error aks.kubeContext not found in $(SITE): copy deploy/azure/site.example.yaml to deploy/azure/site.yaml and fill it in))
KUBECTL := kubectl --context $(KUBE_CONTEXT)
PROFILE_DIR := profiles/$(PROFILE)

# The last `make images` build, if any, is in .images.env (IMAGE_REGISTRY and
# IMAGE_TAG). `make demos` reads it when its recipe runs, not when make starts,
# so `make images demos` deploys the build it just made.
IMAGE_ENV := .images.env

# name,build context,Dockerfile — the images publish.yml builds, by repository.
IMAGES := \
  omnia-demo-memory-ingestion,memory-ingestion,memory-ingestion/Dockerfile \
  omnia-demo-memory-seeder,.,services/memory-seeder/Dockerfile \
  omnia-demo-mortgage-underwriting,.,services/mortgage-underwriting/Dockerfile \
  omnia-demo-personal-agent,.,services/personal-agent/Dockerfile \
  omnia-demo-sharepoint-adapter,.,services/sharepoint-adapter/Dockerfile \
  omnia-demo-sites,.,services/demo-sites/Dockerfile \
  omnia-demo-analyst-sandbox,services/analyst-sandbox,services/analyst-sandbox/Dockerfile

.PHONY: demos builder lint images azure-tenant azure-tenant-remove azure-identity azure-poppy azure-memory omnia-values

# A tenant's Azure side on your own Omnia install, described by
# deploy/azure/site.yaml (see site.example.yaml). DRY_RUN=1 prints every write.
#
#   make azure-tenant WS=<tenant>          identity, Foundry access, workspace chart
#   make azure-tenant-remove WS=<tenant>   uninstall it; databases, content and identity stay
WS ?=

azure-tenant:
	@test -n "$(WS)" || { echo "usage: make azure-tenant WS=<tenant>" >&2; exit 1; }
	@deploy/azure/tenant.sh $(WS)

azure-tenant-remove:
	@test -n "$(WS)" || { echo "usage: make azure-tenant-remove WS=<tenant>" >&2; exit 1; }
	@deploy/azure/tenant.sh --remove $(WS)

# Federate the tenant's agents and workers to its identity; run it again after
# adding agents. Prunes credentials of agents that are gone.
azure-identity:
	@test -n "$(WS)" || { echo "usage: make azure-identity WS=<tenant>" >&2; exit 1; }
	@deploy/azure/identity.sh $(WS)

# The Poppy demo's Azure side: DNS records for its hosts, the Key Vault key the
# account tools seal tokens with, and Key Vault Crypto User on it for each
# tenant whose profile uses it. Prints the vault URL for the profiles.
azure-poppy:
	@deploy/azure/poppy.sh

# Institutional memory's Azure side: a private Blob container per memory tenant,
# its documents from memory-documents/<tenant>/ (only changed files upload; files
# removed here are deleted there), and Storage Blob Data Reader on the container
# for the tenant's identity. A no-op without a `memory` block in site.yaml. Run
# it before or after `make demos`; the CronJob it triggers comes from demos.
azure-memory:
	@deploy/azure/memory.sh

# The Omnia Helm values the demo needs on your install (trusted issuers, default
# exposure), from deploy/azure/site.yaml into out/omnia-values.yaml. Prints the
# hosts your install's certificate must cover.
omnia-values:
	@deploy/azure/omnia-values.sh

# Per tenant: deploy/azure/demos.sh makes its Azure side, installs the chart with
# the profile and site-values.sh's overlay, and federates its agents.
demos:
	@set -euo pipefail; \
	test "$(PROFILE)" = azure || { echo "only the azure profile is wired: PROFILE=$(PROFILE)" >&2; exit 1; }; \
	for t in $(TENANTS); do \
	  CHART=$(CHART) PROFILE_DIR=$(PROFILE_DIR) IMAGE_ENV=$(IMAGE_ENV) deploy/azure/demos.sh "$$t"; \
	done; \
	echo "==> Demo tenants installed: $(TENANTS)"

images:
	@IMAGE_ENV=$(IMAGE_ENV) deploy/azure/images.sh $(IMAGES)

# The eval-gated canary on Larkmere Mutual's member assistant (agent pii-chat,
# values piiChat.canary).
#
#   make canary VERSION=2.0   a good candidate: promotes
#   make canary VERSION=2.1   a regression (promises refunds): the judge fails it and it rolls back
#   make canary-status        where the rollout is
#   make canary-reset         back to 1.0.0, callers stopped
#
# The rollout runs on the namespace's ambient waypoint (mesh routing), so each
# step's weight is exact and each conversation, one WebSocket, stays on the
# version it opened on. At every weight an analysis step reads the candidate's
# average judge score (RolloutAnalysis card-support-gate), and a failed
# gate rolls back on its own. The rollout names the namespace's waypoint
# (mesh.waypoint, CANARY_WAYPOINT), so the controller labels the Service to use
# it, as Omnia's rollout docs intend; `make azure-tenant` makes the
# waypoint and labels the namespace the same way. (Omnia #3952, which emptied
# the Service when this was set, is fixed in #3995.)
# Not in the chart: a promotion rewrites
# spec.promptPackRef, and a chart-owned rollout would restart on the next
# upgrade.
CANARY_NS    ?= larkmere-mutual
CANARY_AGENT ?= pii-chat
CANARY_WAYPOINT ?= waypoint
VERSION      ?= 2.0

.PHONY: canary canary-status canary-reset
canary:
	@set -euo pipefail; \
	slug=$$(tr . - <<< "$(VERSION)"); \
	$(KUBECTL) -n $(CANARY_NS) get promptpack "pii-chat-pack-v$$slug" -o name >/dev/null; \
	: "One gate for every version and run: it counts only this agent's candidate"; \
	: "results since the rollout started (Omnia #3997)."; \
	analysis="rolloutanalysis/card-support-gate"; \
	evaldef=$$($(KUBECTL) -n $(CANARY_NS) get $$analysis -o jsonpath='{.spec.metrics[0].provider.arenaEval.evalDef}'); \
	gate="{\"analysis\":{\"templateName\":\"card-support-gate\"}}"; \
	: "The candidate is the same pack (packName pii-chat-pack) at the new version."; \
	patch=$$(jq -n -c --arg ver "$(VERSION).0" --arg wp "$(CANARY_WAYPOINT)" --argjson gate "$$gate" '{spec: {rollout: { \
	  candidate: {promptPackRef: {name: "pii-chat-pack", version: $$ver}}, \
	  steps: [{setWeight: 20}, $$gate, {setWeight: 50}, $$gate, {setWeight: 100}, $$gate], \
	  rollback: {mode: "automatic"}, \
	  trafficRouting: {mode: "mesh", mesh: {waypoint: $$wp}}}}}'); \
	echo "==> Rolling out member assistant $(VERSION).0 to $(CANARY_AGENT): 20%, 50%, 100%, gated by the judge ($$evaldef)"; \
	$(KUBECTL) -n $(CANARY_NS) patch agentruntime $(CANARY_AGENT) --type merge -p "$$patch" >/dev/null; \
	$(KUBECTL) -n $(CANARY_NS) scale deploy card-callers --replicas=1 >/dev/null; \
	echo "==> Callers started. Watch: make canary-status, or the agent's rollout in the dashboard"

canary-status:
	@$(KUBECTL) -n $(CANARY_NS) get agentruntime $(CANARY_AGENT) -o json | jq '{ \
	  promptPack: .spec.promptPackRef, \
	  candidate: .spec.rollout.candidate.promptPackRef, \
	  rollout: .status.rollout, \
	  condition: (.status.conditions[]? | select(.type == "RolloutActive") | {status, reason, message})}'

canary-reset:
	@set -euo pipefail; \
	$(KUBECTL) -n $(CANARY_NS) scale deploy card-callers --replicas=0 >/dev/null; \
	: "End a live rollout the documented way: a candidate equal to the spec is a"; \
	: "rollback, and the controller returns its routing to stable and removes the"; \
	: "canary. Dropping spec.rollout instead leaves the operator-owned"; \
	: "VirtualService sending the last weight to a canary with no pods (503s)."; \
	if [[ -n "$$($(KUBECTL) -n $(CANARY_NS) get agentruntime $(CANARY_AGENT) -o jsonpath='{.spec.rollout.candidate}')" ]]; then \
	  ref=$$($(KUBECTL) -n $(CANARY_NS) get agentruntime $(CANARY_AGENT) -o jsonpath='{.spec.promptPackRef}'); \
	  $(KUBECTL) -n $(CANARY_NS) patch agentruntime $(CANARY_AGENT) --type merge \
	    -p "{\"spec\":{\"rollout\":{\"candidate\":{\"promptPackRef\":$$ref}}}}" >/dev/null; \
	  until ! $(KUBECTL) -n $(CANARY_NS) get deploy $(CANARY_AGENT)-canary >/dev/null 2>&1; do sleep 5; done; \
	fi; \
	$(KUBECTL) -n $(CANARY_NS) patch agentruntime $(CANARY_AGENT) --type merge \
	  -p '{"spec":{"rollout":null,"promptPackRef":{"name":"pii-chat-pack","version":"1.0.0"}}}' >/dev/null; \
	: "Wait for the stable Deployment to settle: a canary started while it is still"; \
	: "rolling (2.0.0 back to 1.0.0) raced it."; \
	sleep 3; \
	$(KUBECTL) -n $(CANARY_NS) rollout status deploy/$(CANARY_AGENT) --timeout=300s >/dev/null; \
	echo "==> $(CANARY_AGENT) back on 1.0.0, no rollout, callers stopped"

# Halvern's research run on a budget (demo 10): the ScheduledFunction
# research-run fires the function every 30s until the workspace's daily budget
# refuses it. Explicitly suspended at rest, so the budget isn't spent early.
RESEARCH_NS ?= halvern-research

.PHONY: research-start research-stop
research-start:
	@$(KUBECTL) -n $(RESEARCH_NS) patch scheduledfunction research-run --type merge -p '{"spec":{"suspend":false}}' >/dev/null; \
	echo "==> research-run firing every 30s; watch Costs (Halvern Partners Research). Stop: make research-stop"

research-stop:
	@$(KUBECTL) -n $(RESEARCH_NS) patch scheduledfunction research-run --type merge -p '{"spec":{"suspend":true}}' >/dev/null; \
	echo "==> research-run suspended"

# Build, test and ship an agent (demo 12): put Halvern back as it was before a
# take. Deletes the take's project (hosted, id contract-triage-<8 chars>, so
# not the contract-triage-dryrun or -seed fallbacks), everything Omnia made
# for it (dev agent, deployed packs, all labelled with the project id), and the
# deployed agent contract-triage, then prunes the agents' cloud trust. The
# same as Delete Project and deleting the agent in the dashboard.
BUILD_LOOP_NS    ?= halvern-partners
BUILD_LOOP_AGENT ?= contract-triage

.PHONY: build-loop-reset
build-loop-reset:
	@set -euo pipefail; \
	ids=$$($(KUBECTL) -n $(BUILD_LOOP_NS) get arenaprojects -o name | sed 's|.*/||' | grep -E '^$(BUILD_LOOP_AGENT)-[a-z0-9]{8}$$' || true); \
	for id in $$ids; do \
	  echo "==> deleting project $$id and what Omnia made for it"; \
	  $(KUBECTL) -n $(BUILD_LOOP_NS) delete agentruntime,promptpacksource,promptpack -l "omnia.altairalabs.ai/arena-project=$$id" --ignore-not-found >/dev/null; \
	  $(KUBECTL) -n $(BUILD_LOOP_NS) delete arenaproject "$$id" --ignore-not-found >/dev/null; \
	done; \
	if $(KUBECTL) -n $(BUILD_LOOP_NS) get agentruntime $(BUILD_LOOP_AGENT) >/dev/null 2>&1; then \
	  echo "==> deleting agent $(BUILD_LOOP_AGENT)"; \
	  $(KUBECTL) -n $(BUILD_LOOP_NS) delete agentruntime $(BUILD_LOOP_AGENT) >/dev/null; \
	fi; \
	deploy/azure/identity.sh $(BUILD_LOOP_NS); \
	echo "==> $(BUILD_LOOP_NS) ready for a take"

# The Arena builder (chart and version from site.yaml builder.*), then the
# tenant's agent federation.
builder:
	@deploy/azure/builder.sh $(BUILDER_TENANT) $(BUILDER_PROVIDER)

# Every tenant must render with the profile and the site's values, so lint needs a
# site file: make lint SITE=deploy/azure/testdata/site-full.yaml
LINT_VALUES := out/lint-site-values.yaml
lint:
	@helm lint $(CHART)
	@set -euo pipefail; mkdir -p out; \
	for t in $(TENANTS); do \
	  deploy/azure/site-values.sh "$$t" > "$(LINT_VALUES)"; \
	  helm template "$$t" $(CHART) -f $(PROFILE_DIR)/common.yaml -f "$(PROFILE_DIR)/$$t.yaml" -f "$(LINT_VALUES)" --set namespace="$$t" \
	    --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/client-id=lint" \
	    --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/tenant-id=lint" >/dev/null; \
	  echo "$$t renders"; \
	done
