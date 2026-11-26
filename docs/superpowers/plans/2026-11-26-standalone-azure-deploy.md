# omnia-demo standalone Azure deploy: implementation plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** `make demos` deploys every tenant onto any Omnia-on-AKS install, given
`deploy/azure/site.yaml`, with no omnia-azure checkout. omnia-azure names no
demo afterwards.

**Architecture:** The logic lives in bash scripts under `deploy/azure/`, which
read `site.yaml` with `yq`. Thin Makefile targets wrap them. omnia-azure's
`omnia-workspace` chart is ported, rewritten, to `charts/omnia-workspace`.
Go tests run the scripts with fake `az`, `kubectl` and `helm` on `PATH`, which
log their arguments and answer from canned JSON. Then they assert on that log.
Two final tasks, run by the controller rather than a subagent, verify on the
live AKS deployment that the new path is a no-op and then cut over.

**Tech Stack:** bash, mikefarah `yq` v4, Helm 3, Go 1.x tests (`go test ./...`),
`az` CLI.

**Spec:** `docs/superpowers/specs/2026-11-26-standalone-azure-deploy-design.md`

## Global Constraints

- **No omnia-azure dependency in omnia-demo.** omnia-demo never references
  omnia-azure: no path, no `OMNIA_AZURE`, no target name, no output-file schema.
- **No real values committed.**
  - `deploy/azure/site.yaml` is gitignored.
  - `deploy/azure/site.example.yaml` holds placeholders only (`example.com`,
    `00000000-0000-0000-0000-000000000000`).
  - No tenant value of ours is committed anywhere in omnia-demo: the
    `0ad6a315…` group, `kv-omnia-demo-p-gotzfbhj`, `azure.altairalabs.ai`,
    `dev-kvf5kfvufbv6y3p6`, the Auth0 client IDs, `acromniademo…`.
- **Missing fields.** A required `site.yaml` field that is missing fails with
  `site.yaml: <dotted.path> is required` and exit code 1.
- **Named contexts.** Every `kubectl` call passes `--context <aks.kubeContext>`.
  Every `helm` call passes `--kube-context <aks.kubeContext>`.
- **Subscriptions.** Every `az` call that reaches a subscription passes
  `--subscription <subscription>`. DNS calls pass `dns.subscription`.
- **Dry runs.** With `DRY_RUN=1`, every write prints `DRY_RUN: <command>` and is
  not executed. Reads still run. Writes are `az … create|delete|update`,
  `az rest --method put`, `kubectl apply|annotate|label|delete`, and
  `helm upgrade|install|uninstall`.
- **Names the live deployment already uses.** Keep these exactly:
  - identity: `<identityNamePrefix>-<tenant>`;
  - Helm release: `workspace-<tenant>` in the Omnia namespace;
  - databases: `omnia_sessions_<slug>`, `omnia_memory_<slug>`,
    `omnia_consent_<slug>`;
  - Secrets: `omnia-postgres`, `omnia-postgres-memory`, `omnia-postgres-privacy`;
  - ConfigMap: `omnia-workspace`;
  - federated credentials: `<tenant>-<sa>`;
  - Key Vault key: `omnia-connections`.
- **Only fixed rights.** Azure rights go only to the identities and
  ServiceAccounts these scripts name, plus each AgentRuntime's
  `<name>-facade`. They are never derived from anything else declared on the
  cluster.
- **Scripts.**
  - Every script starts with `set -euo pipefail`.
  - No `$()` in Makefile recipes that call scripts; scripts may use it.
  - Each stand-in comment rule in AGENTS.md still applies.
- **Commits.** Conventional, signed off (`git commit -s`). Each message ends
  with `Claude-Session: https://claude.ai/code/session_01RU5akNxY21E8VsfdQA2Rk3`.

## Review Focus

1. **Re-running is a no-op.** Running a target on a tenant that already exists
   makes no create, delete or role-assignment call. Tasks 3, 4 and 5 test it
   with fakes that report existing resources.
2. **An empty AgentRuntime list never prunes everything.** If listing
   AgentRuntimes fails, `azure-identity` aborts before pruning: an unreachable
   cluster must not delete every `*-facade` credential. Task 4 tests it.
3. **Optional blocks really are optional.** A `site.yaml` without `voice`,
   `nfs` or `images` works, and the related steps are skipped. Tasks 1 and 3
   test it.
4. **Context and subscription on every call.** Every logged `kubectl` or `helm`
   call names the context, and every logged `az` call names the subscription.
   It is one shared harness assertion, applied in every script test.
5. **`DRY_RUN=1` makes no writes.** Under `DRY_RUN=1` the fakes record no write
   verb. It is one harness assertion, applied in every script test.

---

### Task 1: site.yaml, the script library and the test harness

**Files:**
- Create: `deploy/azure/site.example.yaml`, `deploy/azure/lib.sh`,
  `deploy/azure/azure_test.go`, `deploy/azure/testdata/bin/{az,kubectl,helm}`,
  `deploy/azure/testdata/site-full.yaml`, `deploy/azure/testdata/site-min.yaml`
- Modify: `.gitignore` (add `deploy/azure/site.yaml`, `out/`),
  `.github/workflows/ci.yml` (install `yq` v4 before `go test ./...`)

**Interfaces:**
- Produces (lib.sh, sourced by every script):
  - `SITE` is the path to site.yaml, defaulting to `deploy/azure/site.yaml`.
  - `site_get <path>` prints the value, or empty when absent.
  - `site_req <path>` prints the value, or fails with the constraint's message.
  - `site_has <path>` returns 0 when the key is present and non-null.
  - `run <cmd…>` executes the command, or prints `DRY_RUN: <cmd>` when
    `DRY_RUN=1`. Every write goes through it.
  - `AZ` is the array `(az --subscription "$(site_req subscription)")`.
  - `KUBECTL` is the array `(kubectl --context "$(site_req aks.kubeContext)")`.
  - `HELM` is the array `(helm --kube-context "$(site_req aks.kubeContext)")`.
- Produces (azure_test.go, used by Tasks 3–7):
  - `runScript(t, script string, env map[string]string, fixtures map[string]string) (log []string, out string, err error)`
    runs the script with `PATH` set to `testdata/bin` first. The fakes append
    `<tool> <args>` to `$FAKE_LOG` and answer from `$FAKE_FIXTURES/<key>.json`,
    where the key is the args joined with `_`, or a glob-matched fallback.
    They exit 3 when `$FAKE_FAIL` matches the args.
  - `assertContexts(t, log)` checks Review Focus 4.
  - `assertNoWrites(t, log)` checks Review Focus 5.
- `site.example.yaml` holds every field from the spec's "Inputs" section, each
  with a one-line comment, plus:
  - `poppy.upstream.issuer`;
  - `poppy.companies.<tenant>.{clientID, clientSecretName}`;
  - `images.{registry, loginServer, subscription}`;
  - `builder.{chart, version, sandboxImage}`;
  - `dashboardHost`;
  - `gateway.{name, namespace, sectionName}`;
  - `omnia.{namespace, valuesRoot}`.

- [ ] **Step 1: Write failing tests** `TestSiteReqMissingFieldNamesIt`. A script
  that sources lib.sh and calls `site_req postgres.host` against `site-min.yaml`
  without that field exits 1, and stderr contains
  `site.yaml: postgres.host is required`. `TestDryRunPrintsWrites`: `run az identity create x` under
  `DRY_RUN=1` prints `DRY_RUN: az identity create x` and the fake log is empty.
  `TestSiteHasOptional`: `site_has voice` is false on `site-min.yaml` and true
  on `site-full.yaml`.
- [ ] **Step 2: Run** `go test ./deploy/azure/ -run 'TestSite|TestDryRun' -v`.
  It is expected to fail because lib.sh doesn't exist yet.
- [ ] **Step 3: Implement** lib.sh, the fakes, the harness and both fixture
  sites. Then fill in `site.example.yaml`.
- [ ] **Step 4: Run** `go test ./deploy/azure/ -v`. Expected: PASS.
- [ ] **Step 5: Commit** with the message `feat(deploy): site.yaml and the Azure deploy script library`.

### Task 2: Port the workspace chart

**Files:**
- Create: `charts/omnia-workspace/{Chart.yaml,values.yaml,templates/workspace.yaml,README.md}`,
  `charts/omnia-workspace/workspace_test.go` (or tests in `deploy/azure/`)
- Modify: `.github/workflows/ci.yml` (lint and render `charts/omnia-workspace`)

**Interfaces:**
- Produces: the chart `omnia-workspace`. Its values are the same keys as
  omnia-azure's (`name`, `environment`, `identity.{clientId,tenantId}`,
  `postgres.{host,adminLogin}`, `foundry.{endpoint,region,providers[]}`,
  `voice.{endpoint,region,providers[]}`, `nfs.{enabled,storageAccount,shareName,storageClass,size,rootClaim}`,
  `dbBootstrap.{image,adminSecret}`), plus `postgres.password`, which takes one of two forms:
  - `externalSecret: {store, key}`, which renders the ExternalSecrets as now;
  - `secretRef: {name, key}`, a Secret in the release namespace. This form
    renders a `workspace-<t>-secrets` Job that writes the three tenant
    Secrets with `POSTGRES_CONN`. The Job runs under a ServiceAccount whose
    Role covers only Secrets in the tenant namespace.
- Rewrite the chart in omnia-demo's own words. It is our private code, not
  Omnia's, and the user approved publishing it. Keep every comment that
  explains a why: the subtree Job's uid 65532, the claim the operator adopts,
  and pgvector.

- [ ] **Step 1: Write failing tests** that run `helm template` and assert:
  - `TestWorkspaceChartNames`: with `name: larkmere-mutual`, the output holds
    the Namespace `larkmere-mutual`, the ConfigMap `omnia-workspace`, the
    Secrets or ExternalSecrets named as the constraint gives, and the databases
    `omnia_sessions_larkmere_mutual`, `omnia_memory_larkmere_mutual` and
    `omnia_consent_larkmere_mutual` in the db Job.
  - `TestWorkspaceChartPasswordModes`: `externalSecret` renders 3
    ExternalSecrets and no secrets Job; `secretRef` renders the Job and no
    ExternalSecret; setting both fails.
  - `TestWorkspaceChartOptional`: without `voice.endpoint`, no voice Providers
    render; with `nfs.enabled: false`, there is no PV, PVC or subtree Job.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement the chart.**
- [ ] **Step 4: Run** `go test ./...` and `helm lint charts/omnia-workspace`.
  Expected: PASS.
- [ ] **Step 5: Commit** with the message `feat(chart): omnia-workspace, a tenant's Azure side on an Omnia install`.

### Task 3: `make azure-tenant`

**Files:**
- Create: `deploy/azure/tenant.sh`, `deploy/azure/ambient-waypoint.yaml`
  (rewritten from omnia-azure's `manifests/ambient-waypoint.yaml`, with the
  `__WORKSPACE_NS__` placeholder)
- Modify: `Makefile` (target `azure-tenant WS=<t>` → `deploy/azure/tenant.sh $(WS)`,
  and `azure-tenant-remove WS=<t>` → `helm uninstall workspace-<t>`, which keeps
  the databases, the content and the identity, as `workspace-remove` does)

**Interfaces:**
- Consumes: Task 1's lib.sh and Task 2's chart.
- Produces: `tenant.sh <tenant>`. When it finishes, the namespace exists, along
  with its ConfigMap `omnia-workspace` (`workloadIdentityClientId`,
  `workloadIdentityTenantId`) and the Providers. The steps, in this order:
  1. Create the identity if it's absent.
  2. Grant `Cognitive Services OpenAI User` on `foundry.accountId` if it's
     missing, then on `voice.accountId` when there is one.
  3. Run `helm upgrade --install workspace-<t> charts/omnia-workspace -n <omnia.namespace> --wait --timeout 5m`.
  4. When `kubectl -n istio-system get daemonset ztunnel` succeeds, label the
     namespace and apply the waypoint.

- [ ] **Step 1: Write failing tests:**
  - `TestTenantCreatesIdentityAndGrants`: with the identity missing, the log
    holds `az identity create … -n mi-x-larkmere-mutual` and one
    `az role assignment create … --role Cognitive Services OpenAI User`.
  - `TestTenantIdempotent`: with the identity and grant present, the log holds
    no `create` (Review Focus 1).
  - `TestTenantNoVoiceNoAmbient`: with `site-min.yaml` and the ztunnel lookup
    failing, there is no voice grant, no `label` and no waypoint `apply`
    (Review Focus 3).
  - `TestTenantDryRun`: `assertNoWrites`.
  - Every test calls `assertContexts`.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement** `tenant.sh` and the Makefile target.
- [ ] **Step 4: Run** `go test ./deploy/azure/ -run TestTenant -v`. Expected: PASS.
- [ ] **Step 5: Commit** with the message `feat(deploy): make azure-tenant`.

### Task 4: `make azure-identity`

**Files:**
- Create: `deploy/azure/identity.sh`
- Modify: `Makefile` (target `azure-identity WS=<t>`)

**Interfaces:**
- Produces: `identity.sh <tenant>`.
  - **Federation:** it federates `arena-worker`, `eval-worker`, `memory-<t>-default`,
    `privacy-<t>`, `omnia-identity-foundry` and `<agent>-facade` for each
    AgentRuntime in `<t>`. Each gets the credential name `<t>-<sa>`, the AKS
    OIDC issuer from `az aks show … --query oidcIssuerProfile.issuerUrl`, and
    audience `api://AzureADTokenExchange`. Existing subjects are skipped.
  - **Pruning:** it deletes each `*-facade` credential whose AgentRuntime is gone.
  - **memory-api:** it annotates the SA `memory-<t>-default` with the client and
    tenant ids when they differ, then deletes the memory-api pods (not a
    rollout restart; keep the comment that explains why).
  - **When listing AgentRuntimes fails:** it exits non-zero before any
    federation or prune.

- [ ] **Step 1: Write failing tests:**
  - `TestIdentityFederatesFixedAndAgents`: with 2 AgentRuntimes, the log has
    7 `federated-credential create` calls with the subjects listed above.
  - `TestIdentityPrunesOnlyGoneFacades`: existing credentials for
    `a-facade` (AgentRuntime present), `gone-facade` (absent) and
    `eval-worker` produce exactly one delete, for `gone-facade`.
  - `TestIdentityAbortsWhenAgentListFails`: `FAKE_FAIL` on
    `get agentruntime` gives a non-zero exit and no `delete` or `create` in
    the log (Review Focus 2).
  - `TestIdentityIdempotent`: with all subjects present, there is no write.
  - A dry-run test.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement it.**
- [ ] **Step 4: Run** `go test ./deploy/azure/ -run TestIdentity -v`. Expected: PASS.
- [ ] **Step 5: Commit** with the message `feat(deploy): make azure-identity`.

### Task 5: `make azure-poppy`

**Files:**
- Create: `deploy/azure/poppy.sh`
- Modify: `Makefile` (target `azure-poppy`)

**Interfaces:**
- Produces: `poppy.sh`.
  - **Gateway IP:** from `dns.gatewayIP`, or from the public IP resource
    `dns.gatewayPublicIP.{resourceGroup,name}`.
  - **DNS records:** for each host in the list below, it replaces that host's A
    record in `dns.zone`, using `dns.subscription` and `dns.resourceGroup`
    with TTL 300. The record names are relative: `larkmere`, `wrenmoor`,
    `auth.larkmere`, `auth.wrenmoor` and `assistant`.
  - **The key:** it creates `omnia-connections` (RSA 3072, wrapKey/unwrapKey)
    through `az rest --method put` on
    `https://management.azure.com<vault id>/keys/omnia-connections?api-version=2023-07-01`
    when `az resource show --ids` doesn't find it. Keep the comment that
    explains why: an RBAC-only vault, and no data-plane role for the caller.
  - **Grants:** it grants `Key Vault Crypto User` on that key to
    `<prefix>-<t>` for each tenant `t` whose `profiles/azure/<t>.yaml` sets
    `poppy.account.enabled: true`. It reads that with yq from the repo files,
    never from the cluster.
  - **Output:** it prints `vault-url https://<vault>.vault.azure.net/`.
- The host list lives in one place: `deploy/azure/hosts.sh`'s
  `poppy_dns_names`, which Tasks 6 and 7 reuse.

- [ ] **Step 1: Write failing tests:**
  - `TestPoppyDNS`: the log has 5 `record-set a add-record` calls, each with
    `--subscription <dns.subscription>`.
  - `TestPoppyKeyOnlyWhenMissing`: one `az rest --method put` when the key is
    missing, and none when it is present.
  - `TestPoppyGrantsFromProfiles`: grants go to exactly
    `larkmere-mutual` and `wrenmoor-energy`, the profiles in the repo today.
  - A dry-run test.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement it.**
- [ ] **Step 4: Run them.** Expected: PASS.
- [ ] **Step 5: Commit** with the message `feat(deploy): make azure-poppy`.

### Task 6: `make omnia-values`

**Files:**
- Create: `deploy/azure/omnia-values.sh`
- Modify: `Makefile` (target `omnia-values` → `out/omnia-values.yaml`)

**Interfaces:**
- Produces: `out/omnia-values.yaml`, nested under `omnia.valuesRoot` when it is
  non-empty. It holds:
  - **Trusted issuers.** `operator.delegationAPI.trustedIssuers` gets four
    entries, each `<issuer>=<issuer>/.well-known/jwks.json`. The issuers are
    `http://larkmere-portal.larkmere-mutual.svc.cluster.local:8080`,
    `http://card-callers.larkmere-mutual.svc.cluster.local:8080`,
    `http://wrenmoor-site.wrenmoor-energy.svc.cluster.local:8080` and
    `http://assistant-page.ellens-assistant.svc.cluster.local:8080`.
    Add a comment that each issuer is a stand-in edge (docs/stand-ins.md).
  - **Default exposure.** `defaultExposure`:
    `{enabled: true, baseDomain: <dns.zone>, gateway: <site gateway>}`.
- It also prints `TLS hosts: <space-separated FQDNs>`: the five Poppy hosts and
  `dashboardHost`. This is what the install's certificate must cover.

- [ ] **Step 1: Write failing tests:**
  - `TestOmniaValuesUmbrellaRoot`: with `valuesRoot: omnia`, `yq` reads 4
    entries at `.omnia.operator.delegationAPI.trustedIssuers`.
  - `TestOmniaValuesBareRoot`: with `valuesRoot: ""`, they are at
    `.operator.delegationAPI.trustedIssuers`.
  - `TestOmniaValuesTLSHosts`: stdout lists exactly the 6 FQDNs.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement it.**
- [ ] **Step 4: Run them.** Expected: PASS.
- [ ] **Step 5: Commit** with the message `feat(deploy): make omnia-values`.

### Task 7: `make demos` and the profiles without our tenant

**Files:**
- Create: `deploy/azure/site-values.sh` (prints one tenant's overlay) and
  `deploy/azure/builder.sh`
- Modify: `Makefile` (the `demos`, `images`, `builder` and `build-loop-reset`
  targets and the header comment; drop `OMNIA_AZURE`), every file in
  `profiles/azure/`

**Interfaces:**
- Produces: `site-values.sh <tenant>`, which writes YAML to stdout. It holds
  exactly the keys that the profiles stop setting:
  - `workspace.roleBindings` from `ownerGroups`;
  - every host built from `dns.zone`: the Company `domain`, `allow`, `issuer`
    and `audiences`/`audience`; the assistant's `host` and Companies;
  - the catalogue route's `parentRefs`, from `gateway`, and its `hostnames`,
    from `dashboardHost`;
  - `poppy.auth.gateway.upstream.{issuer, clientID}` from `poppy.upstream`;
  - `encryption.vaultURL` from `keyVault.name`.

  Find each key by grepping `profiles/azure/` for the committed values that
  the Global Constraints forbid. After this task, that grep finds nothing.
- `make demos`, per tenant, in this order:
  1. run `tenant.sh`;
  2. read the client and tenant ids from the ConfigMap;
  3. run `helm upgrade --install` with `-f common.yaml -f <t>.yaml -f <(site-values.sh t)`,
     the same `--set` flags and the images from `.images.env`;
  4. run `identity.sh`.
- `make images` reads the registry from `images.*`.
- `make builder` runs `builder.sh <tenant> <provider>`, which installs the chart
  given by `builder.chart`/`builder.version` (with `sandboxImage` when it is
  set) as release `arena-builder`, then runs `identity.sh`.
- `make build-loop-reset` ends with `identity.sh` in place of the call to
  omnia-azure.

- [ ] **Step 1: Write failing tests:**
  - `TestSiteValuesHasNoCommittedTenant`: running `git grep` for every
    forbidden value from the Global Constraints over `profiles/ charts/ Makefile`
    finds nothing.
  - `TestRenderedTenantsUnchanged`: build a site from our real values (in a
    test-only, gitignored fixture, or skip when it is missing). For each
    tenant, `helm template` of `common.yaml` + profile + `site-values.sh` equals
    `helm template` of the profile at the base commit.
  - `TestBuilderUsesSiteChart`: the log has one `helm upgrade --install
    arena-builder <builder.chart> --version <builder.version>` call, followed
    by identity's calls.
- [ ] **Step 2: Run them and see them fail.**
- [ ] **Step 3: Implement it.** Run `make lint` with a test `SITE`.
- [ ] **Step 4: Run** `go test ./...` and `make lint SITE=deploy/azure/testdata/site-full.yaml`.
  Expected: PASS.
- [ ] **Step 5: Commit** with the message `feat(deploy): make demos needs only site.yaml`.

### Task 8: Documentation

**Files:**
- Create: `deploy/azure/README.md`
- Modify: `README.md`, `demos/personal-agent/RUNBOOK.md`, `docs/stand-ins.md`
  and any other file that `git grep -n omnia-azure` finds

**Interfaces:**
- `deploy/azure/README.md` covers:
  - **What it needs:** an Omnia install on AKS with workload identity, a
    Postgres Flexible Server, a Foundry account, a DNS zone, a Key Vault,
    cert-manager or another TLS source, and optionally NFS and an ACR. List
    the Azure rights the caller needs for each.
  - **First install, in order:**
    1. write `site.yaml`;
    2. run `make omnia-values` and add its output and TLS hosts to the Omnia
       install;
    3. run `make azure-poppy`;
    4. run `hack/personal-agent-keys.sh`;
    5. run `make images`;
    6. run `make demos`.
  - **Day-two operations:** after adding an agent, run `make azure-identity`.
- Afterwards, `git grep -n omnia-azure` in omnia-demo matches only a sentence
  saying the demo was first run on our deployment, or nothing.

- [ ] **Step 1: Write the docs.**
- [ ] **Step 2: Check** that `git grep -n -i 'omnia-azure\|OMNIA_AZURE'` finds
  nothing operational.
- [ ] **Step 3: Commit** with the message `docs: deploying the demos onto your own Omnia on AKS`.

### Task 9 (controller, live AKS): verify the new path is a no-op

The controller runs this task, not a subagent. Every command names
`--context aks-omnia-demo`.

- [ ] **Step 1:** Write our `deploy/azure/site.yaml` from omnia-azure's
  `.bootstrap-output.json`, `.deploy-output.json` and `.registry.json`, and
  from the current profiles' values. It stays local.
- [ ] **Step 2:** For each tenant, render-diff `helm template` of omnia-azure's
  `charts/omnia-workspace` against omnia-demo's, both with the values that
  `tenant.sh` passes.
  - Expected: labels or chart version only. A change to any other spec field
    is a stop.
- [ ] **Step 3:** Run `DRY_RUN=1 make azure-tenant`, `azure-identity` and
  `azure-poppy` for every tenant.
  - Expected: no `DRY_RUN:` lines other than `helm upgrade`.
- [ ] **Step 4:** Check the generated Omnia values against the install:
  `make omnia-values`'s `trustedIssuers` equals
  `helm --kube-context aks-omnia-demo -n omnia get values omnia`'s.
- [ ] **Step 5:** Run `make demos` for real.
  - Expected: no Azure create or delete in its output.
  - Every tenant comes up healthy, and the Poppy metadata still verifies, with
    PoppyIssuerGateway=True and PoppyIssuer=True on both Companies.
- [ ] **Step 6:** Record the result in the ledger.

### Task 10 (controller): omnia-azure stops naming the demo

Done in the omnia-azure worktree `omnia-azure-wt-poppy-hosts`, on branch
`feat/poppy-hosts`, as new commits.

- [ ] **Step 1:** Add `EXTRA_VALUES` (a space-separated list of `-f` files) and
  `TLS_EXTRA_HOSTS` (which replaces the `poppyHosts` value with
  `extraDnsNames`) to `make install`. Add `make outputs`, which prints the
  deployment's facts as JSON.
- [ ] **Step 2:** Remove the following:
  - `poppyHosts`, `poppy-dns` (and the `dns` target's dependency on it),
    `poppy-kms` and `POPPY_*`;
  - the trusted issuers and the defaultExposure Poppy comments, which now
    come from `EXTRA_VALUES`;
  - `workspace`, `workspace-identity`, `workspace-remove` and
    `charts/omnia-workspace`;
  - `builder`, `manifests/ambient-waypoint.yaml` (unless `ambient` uses it),
    `demo-content`, `bicep/scripts/deploy-demo-content.sh` and
    `demo-content/`.

  Then fix the help text and docs.
- [ ] **Step 3:** Run `make install EXTRA_VALUES=../omnia-demo/out/omnia-values.yaml TLS_EXTRA_HOSTS="…"`
  with the printed hosts. First confirm that the `helm diff` of the release
  shows no change.
- [ ] **Step 4:** Verify each of the following:
  - the dashboard and the catalogue load;
  - `curl` of each Poppy host's discovery and metadata documents returns 200;
  - the trusted issuers are unchanged.
- [ ] **Step 5:** Commit, then `git grep -n -i -E 'larkmere|wrenmoor|halvern|ellen|poppy'`
  in omnia-azure. Expected: nothing.
