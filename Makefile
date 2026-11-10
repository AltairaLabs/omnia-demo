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
#
# Requires a sibling checkout of omnia-azure (OMNIA_AZURE) with its deploy
# outputs, and kubectl/helm access to that cluster.

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

.PHONY: demos builder lint

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
	    --set namespace="$$t" \
	    --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/client-id=$$client_id" \
	    --set-string "workspace.runtimeIdentity.serviceAccountAnnotations.azure\.workload\.identity/tenant-id=$$tenant_id" \
	    --wait --timeout 10m >/dev/null; \
	  $(MAKE) --no-print-directory -C "$(OMNIA_AZURE)" workspace-identity WS="$$t"; \
	done; \
	echo "==> Demo tenants installed: $(TENANTS)"

builder:
	@$(MAKE) --no-print-directory -C "$(OMNIA_AZURE)" builder BUILDER_TARGETS="$(BUILDER_TENANT):$(BUILDER_PROVIDER)"

lint:
	@helm lint $(CHART)
	@for t in $(TENANTS); do helm template "$$t" $(CHART) -f $(PROFILE_DIR)/common.yaml -f "$(PROFILE_DIR)/$$t.yaml" --set namespace="$$t" >/dev/null && echo "$$t renders"; done
