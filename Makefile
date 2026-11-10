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

# The last `make images` build, if any: IMAGE_REGISTRY and IMAGE_TAG.
-include .images.env
IMAGE_SET := $(if $(IMAGE_REGISTRY),--set-string images.registry=$(IMAGE_REGISTRY) --set-string images.tag=$(IMAGE_TAG))

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
	for t in $(TENANTS); do \
	  echo "==> $$t"; \
	  $(MAKE) --no-print-directory -C "$(OMNIA_AZURE)" workspace WS="$$t"; \
	  client_id=$$($(KUBECTL) -n "$$t" get configmap omnia-workspace -o jsonpath='{.data.workloadIdentityClientId}'); \
	  tenant_id=$$($(KUBECTL) -n "$$t" get configmap omnia-workspace -o jsonpath='{.data.workloadIdentityTenantId}'); \
	  $(HELM) upgrade --install "$$t" $(CHART) -n "$$t" \
	    -f $(PROFILE_DIR)/common.yaml -f "$(PROFILE_DIR)/$$t.yaml" \
	    --set namespace="$$t" $(IMAGE_SET) \
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
	printf 'IMAGE_REGISTRY := %s\nIMAGE_TAG := %s\n' "$$server" "$$tag" > .images.env; \
	echo "==> Built $$tag; make demos now runs it (rm .images.env to go back)"

builder:
	@$(MAKE) --no-print-directory -C "$(OMNIA_AZURE)" builder BUILDER_TARGETS="$(BUILDER_TENANT):$(BUILDER_PROVIDER)"

lint:
	@helm lint $(CHART)
	@for t in $(TENANTS); do helm template "$$t" $(CHART) -f $(PROFILE_DIR)/common.yaml -f "$(PROFILE_DIR)/$$t.yaml" --set namespace="$$t" >/dev/null && echo "$$t renders"; done
