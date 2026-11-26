# omnia-workspace

The Azure side of one Omnia workspace, one Helm release per workspace
(`workspace-<name>`, installed into Omnia's namespace). It renders:

- the tenant Namespace and an `omnia-workspace` ConfigMap describing it;
- three database Secrets (`omnia-postgres`, `omnia-postgres-memory`,
  `omnia-postgres-privacy`, each holding `POSTGRES_CONN`);
- a pre-install Job that creates the workspace's own three databases
  (`omnia_sessions_<slug>`, `omnia_memory_<slug>`, `omnia_consent_<slug>`);
- keyless Foundry Providers, and voice Providers when `voice.endpoint` is set;
- when `nfs.enabled`, the workspace-content PV and PVC and a Job that creates
  the workspace's subtree of the share.

It knows nothing about what runs in the workspace.

## Where the database password comes from

Set exactly one of these under `postgres.password` (neither or both fails the
render):

- `externalSecret: {store, key}`: the three Secrets are ExternalSecrets read
  from a `ClusterSecretStore`. Needs the External Secrets operator.
  `key` defaults to `omnia-postgres-admin-password`.
- `secretRef: {name, key}`: a Secret in the release namespace. A post-install
  Job (`workspace-<name>-secrets`, in the release namespace) takes the
  password from it as an env var and writes the three Secrets through stdin.
  The password is URL-encoded in `POSTGRES_CONN` (the ExternalSecret form is
  not). Its ServiceAccount's only rights are a Role in the tenant namespace
  limited to Secrets. The databases Job reads this same Secret and key instead
  of `dbBootstrap.adminSecret`. `key` must be a valid Secret key. Set
  `secretsJob.image` to any image with `kubectl`, `jq` and `sh`.

## Try it

    helm lint charts/omnia-workspace --set name=larkmere-mutual \
      --set postgres.password.externalSecret.store=azure-kv
    helm template workspace-larkmere-mutual charts/omnia-workspace -n omnia \
      --set name=larkmere-mutual --set nfs.enabled=false \
      --set postgres.password.secretRef.name=omnia-db-admin \
      --set postgres.password.secretRef.key=PGPASSWORD

See `values.yaml` for every value.
