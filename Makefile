# Install the demo tenants into an Omnia install.
#
# One Workspace per company in the fictional cast, each an omnia-demo release
# in its own namespace. The platform-specific values live in profiles/<PROFILE>/
# (common.yaml under one file per tenant). The azure profile installs onto an
# omnia-azure deployment: it asks omnia-azure to provision each workspace's Azure
# side (`make workspace`), installs the chart, then has it federate the agents
# (`make workspace-identity`).
#
#   make demos                          every tenant
#   make demos TENANTS=wrenmoor-energy  one tenant
#   make builder                        the Arena builder, for Halvern Partners
#   make images                         build every image into the dev registry
#
# `make images` builds each demo image in omnia-azure's dev registry (its
# `make registry`) with `az acr build`, tagged dev-<commit>, and records the
# registry and tag in .images.env. While that file exists, `make demos` runs
# every image from that build instead of the azure profile's pins; delete it to
# go back. Nothing is published from this repository.
#
# Requires a sibling checkout of omnia-azure (OMNIA_AZURE) with its deploy
# outputs, and kubectl/helm access to that cluster.

SHELL := bash

PROFILE          ?= azure
TENANTS          ?= larkmere-mutual wrenmoor-energy halvern-partners
KUBE_CONTEXT     ?= aks-omnia-demo
OMNIA_AZURE      ?= ../omnia-azure
CHART            ?= charts/omnia-demo
BUILDER_TENANT   ?= halvern-partners
BUILDER_PROVIDER ?= azure-foundry-gpt-5-4-mini

KUBECTL := kubectl --context $(KUBE_CONTEXT)
HELM    := helm --kube-context $(KUBE_CONTEXT)
PROFILE_DIR := profiles/$(PROFILE)

# The last `make images` build, if any, is in .images.env (IMAGE_REGISTRY and
# IMAGE_TAG). `make demos` reads it when its recipe runs, not when make starts,
# so `make images demos` deploys the build it just made.
IMAGE_ENV := .images.env

# name,build context,Dockerfile — the images publish.yml builds, by repository.
IMAGES := \
  omnia-demo-memory-ingestion,memory-ingestion,memory-ingestion/Dockerfile \
  omnia-demo-memory-seeder,.,demos/memory-seeder/Dockerfile \
  omnia-demo-mortgage-underwriting,.,demos/mortgage-underwriting/Dockerfile \
  omnia-demo-sharepoint-adapter,.,demos/sharepoint-adapter/Dockerfile \
  omnia-demo-sites,.,demos/demo-sites/Dockerfile

.PHONY: demos builder lint images

demos:
	@set -euo pipefail; \
	test "$(PROFILE)" = azure || { echo "only the azure profile is wired: PROFILE=$(PROFILE)" >&2; exit 1; }; \
	test -d "$(OMNIA_AZURE)" || { echo "OMNIA_AZURE=$(OMNIA_AZURE) is not a checkout of omnia-azure" >&2; exit 1; }; \
	image_set=""; \
	if [[ -f $(IMAGE_ENV) ]]; then \
	  reg=$$(awk -F' *:?= *' '$$1=="IMAGE_REGISTRY"{print $$2}' $(IMAGE_ENV)); \
	  tag=$$(awk -F' *:?= *' '$$1=="IMAGE_TAG"{print $$2}' $(IMAGE_ENV)); \
	  image_set="--set-string images.registry=$$reg --set-string images.tag=$$tag"; \
	  echo "==> Images: $$reg, tag $$tag ($(IMAGE_ENV))"; \
	fi; \
	for t in $(TENANTS); do \
	  echo "==> $$t"; \
	  $(MAKE) --no-print-directory -C "$(OMNIA_AZURE)" workspace WS="$$t"; \
	  client_id=$$($(KUBECTL) -n "$$t" get configmap omnia-workspace -o jsonpath='{.data.workloadIdentityClientId}'); \
	  tenant_id=$$($(KUBECTL) -n "$$t" get configmap omnia-workspace -o jsonpath='{.data.workloadIdentityTenantId}'); \
	  $(HELM) upgrade --install "$$t" $(CHART) -n "$$t" \
	    -f $(PROFILE_DIR)/common.yaml -f "$(PROFILE_DIR)/$$t.yaml" \
	    --set namespace="$$t" $$image_set \
	    --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/client-id=$$client_id" \
	    --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/tenant-id=$$tenant_id" \
	    --wait --timeout 10m >/dev/null; \
	  $(MAKE) --no-print-directory -C "$(OMNIA_AZURE)" workspace-identity WS="$$t"; \
	done; \
	echo "==> Demo tenants installed: $(TENANTS)"

images:
	@set -euo pipefail; \
	acr=$$(jq -r .name "$(OMNIA_AZURE)/.registry.json" 2>/dev/null || true); \
	server=$$(jq -r .loginServer "$(OMNIA_AZURE)/.registry.json" 2>/dev/null || true); \
	sub=$$(jq -r .subscription "$(OMNIA_AZURE)/.registry.json" 2>/dev/null || true); \
	if [[ -z "$$acr" ]]; then echo "no dev registry: run 'make registry' in $(OMNIA_AZURE)" >&2; exit 1; fi; \
	tag=dev-$$(git rev-parse --short HEAD); \
	if [[ -n "$$(git status --porcelain)" ]]; then tag=$$tag-$$(date +%Y%m%d%H%M); fi; \
	echo "==> Building into $$server, tag $$tag"; \
	for spec in $(IMAGES); do \
	  IFS=, read -r name context file <<< "$$spec"; \
	  echo "==> $$name"; \
	  az acr build --subscription "$$sub" --registry "$$acr" --platform linux/amd64 --image "$$name:$$tag" \
	    --file "$$file" --no-logs --output none "$$context"; \
	done; \
	printf 'IMAGE_REGISTRY := %s\nIMAGE_TAG := %s\n' "$$server" "$$tag" > $(IMAGE_ENV); \
	echo "==> Built $$tag; make demos now runs it (rm .images.env to go back)"

# The eval-gated canary on Larkmere Mutual's card support (piiChat.canary).
#
#   make canary VERSION=2.0   a good candidate: promotes
#   make canary VERSION=2.1   a regression: the judge fails it and it rolls back
#   make canary-status        where the rollout is
#   make canary-reset         back to 1.0.0, callers stopped
#
# The rollout runs on the namespace's ambient waypoint (mesh routing), so each
# step's weight is exact and each conversation, one WebSocket, stays on the
# version it opened on. At every weight an analysis step reads the candidate's
# average judge score (RolloutAnalysis card-support-gate-v<ver>), and a failed
# gate rolls back on its own. mesh.waypoint is left unset on purpose: on Omnia
# 0.9.0-beta.23 it puts istio.io/use-waypoint in the Service's selector, which
# empties the Service (Omnia #3952). The namespace's own use-waypoint label
# (omnia-azure `make workspace`) already sends the Service through the waypoint.
# Not in the chart: a promotion rewrites
# spec.promptPackRef, and a chart-owned rollout would restart on the next
# upgrade.
CANARY_NS    ?= larkmere-mutual
CANARY_AGENT ?= pii-chat
VERSION      ?= 2.0

.PHONY: canary canary-status canary-reset
canary:
	@set -euo pipefail; \
	slug=$$(tr . - <<< "$(VERSION)"); \
	$(KUBECTL) -n $(CANARY_NS) get promptpack "pii-chat-pack-v$$slug" -o name >/dev/null; \
	gate="{\"analysis\":{\"templateName\":\"card-support-gate-v$$slug\"}}"; \
	: "The candidate is the same pack (packName pii-chat-pack) at the new version."; \
	patch=$$(jq -n -c --arg ver "$(VERSION).0" --argjson gate "$$gate" '{spec: {rollout: { \
	  candidate: {promptPackRef: {name: "pii-chat-pack", version: $$ver}}, \
	  steps: [{setWeight: 20}, $$gate, {setWeight: 50}, $$gate, {setWeight: 100}, $$gate], \
	  rollback: {mode: "automatic"}, \
	  trafficRouting: {mode: "mesh"}}}}'); \
	echo "==> Rolling out card support $(VERSION).0 to $(CANARY_AGENT): 20%, 50%, 100%, gated by the judge"; \
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
	echo "==> $(CANARY_AGENT) back on card support 1.0.0, no rollout, callers stopped"

builder:
	@$(MAKE) --no-print-directory -C "$(OMNIA_AZURE)" builder BUILDER_TARGETS="$(BUILDER_TENANT):$(BUILDER_PROVIDER)"

lint:
	@helm lint $(CHART)
	@for t in $(TENANTS); do helm template "$$t" $(CHART) -f $(PROFILE_DIR)/common.yaml -f "$(PROFILE_DIR)/$$t.yaml" --set namespace="$$t" >/dev/null && echo "$$t renders"; done
