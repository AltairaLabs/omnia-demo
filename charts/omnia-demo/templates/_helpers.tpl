{{/*
The Provider an agent uses: llm.providerRef when it is set, so every demo runs
on one Provider that already exists in the namespace (a keyless Azure AI
Foundry Provider, say). Otherwise the demo's own Provider, named by `own`.
A demo's own providerRef (`demoRef`) wins over both, so one demo can use a
Provider tuned for it (longer output, say) while the rest share one.
Call with (dict "root" $ "own" "<demo>-llm" "demoRef" .Values.<demo>.providerRef).
*/}}
{{- define "omnia-demo.llmProvider" -}}
{{- default (default .own .root.Values.llm.providerRef) .demoRef -}}
{{- end -}}

{{/*
An image reference. An empty tag means the chart's appVersion: the publish
workflow tags every image with the release version, so a chart release pulls
the images built with it.
Call with (dict "root" $ "image" .Values.<demo>.image).
*/}}
{{- define "omnia-demo.image" -}}
{{- $override := .root.Values.images | default dict -}}
{{- $repository := .image.repository -}}
{{- if $override.registry -}}
{{- $repository = printf "%s/%s" (trimSuffix "/" $override.registry) (base .image.repository) -}}
{{- end -}}
{{- $repository }}:{{ coalesce $override.tag .image.tag .root.Chart.AppVersion -}}
{{- end -}}

{{/* The demo Workspace's name: workspace.name, or the namespace. */}}
{{- define "omnia-demo.workspaceName" -}}
{{- default .Values.namespace .Values.workspace.name -}}
{{- end -}}

{{/*
The workspace memory-api: memory.apiURL when set, otherwise the managed
service group's memory-api, which the operator names
memory-<workspace>-<group> in the workspace namespace.
*/}}
{{- define "omnia-demo.memoryURL" -}}
{{- if .Values.memory.apiURL -}}
{{- .Values.memory.apiURL -}}
{{- else -}}
http://memory-{{ include "omnia-demo.workspaceName" . }}-{{ .Values.workspace.serviceGroup }}.{{ .Values.namespace }}.svc.cluster.local:8080
{{- end -}}
{{- end -}}

{{/*
The ambient-retrieval filter that keeps only Larkmere's guidance for members:
documents under member-guidance/ in the memory bucket. The same institutional
tier holds the society's claims-handling rules, which an agent that speaks to
members must never read. An item with no source_ref (the Galaxy's) is dropped.
*/}}
{{- define "omnia-demo.memberGuidanceOnly" -}}
!(has(metadata.source_ref) && metadata.source_ref.contains("/member-guidance/"))
{{- end -}}

{{/* The Galaxy's MemoryPolicy (cluster-scoped): galaxy.policyName, or <workspace>-memory. */}}
{{- define "omnia-demo.galaxyPolicyName" -}}
{{- default (printf "%s-memory" (include "omnia-demo.workspaceName" .)) .Values.galaxy.policyName -}}
{{- end -}}

{{/*
The `iss` of a front door's identity provider (demos/demoidp), and the base of
its JWKS URL (<issuer>/.well-known/jwks.json): the in-cluster address of its
Service, which is where Omnia's mint fetches the keys. The install lists each
in the operator's delegation.trustedIssuers.
*/}}
{{- define "omnia-demo.issuerURL" -}}
http://{{ .service }}.{{ .root.Values.namespace }}.svc.cluster.local:8080
{{- end }}

{{/*
A card-support pack.json (1.0.0 or a canary candidate) as the install serves
it. With piiChat.guardrail off, the scope guardrail and the `guard` slot it
names are dropped rather than shipped unbound: an unbound topic_policy fails
closed and the assistant would refuse every turn. See piiChat.guardrail.
Call with (dict "root" $ "file" "files/arena/<pack>.json").
*/}}
{{- define "omnia-demo.piiChatPackJSON" -}}
{{- $pack := .root.Files.Get .file -}}
{{- if not .root.Values.piiChat.guardrail.enabled -}}
{{- $p := $pack | fromJson -}}
{{- range $id, $prompt := $p.prompts -}}
{{- $kept := list -}}
{{- range ($prompt.validators | default list) -}}
{{- if ne .type "topic_policy" }}{{ $kept = append $kept . }}{{ end -}}
{{- end -}}
{{- if $kept }}{{ $_ := set $prompt "validators" $kept }}{{ else }}{{ $_ := unset $prompt "validators" }}{{ end -}}
{{- end -}}
{{- if $p.requires -}}
{{- $reqs := list -}}
{{- range ($p.requires.providers | default list) -}}
{{- if ne (toString .key) "guard" }}{{ $reqs = append $reqs . }}{{ end -}}
{{- end -}}
{{- $_ := set $p.requires "providers" $reqs -}}
{{- end -}}
{{- $pack = $p | toPrettyJson -}}
{{- end -}}
{{- $pack -}}
{{- end -}}

{{/*
The issuer gateway's own URL (Omnia's Poppy issuer gateway, from dd2e94540):
poppy.auth.issuer, the one source of the gateway's host. Omnia serves the
gateway on the host of auth.issuer and requires it to be `https://` and a host
with no path, not even `/`; anything else fails the render here rather than at
the API server. Call with the root context, only with poppy.auth.gateway.enabled.
*/}}
{{- define "omnia-demo.gatewayIssuer" -}}
{{- $iss := required "poppy.auth.issuer (the gateway's URL, https://<auth host>)" .Values.poppy.auth.issuer -}}
{{- if not (regexMatch "^https://[a-z0-9]([a-z0-9-]*[a-z0-9])?(\\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$" $iss) -}}
{{- fail (printf "poppy.auth.issuer %q: with poppy.auth.gateway.enabled it is the gateway's URL, https:// and a lowercase host with no path and no trailing slash" $iss) -}}
{{- end -}}
{{- $iss -}}
{{- end -}}

{{/*
The issuer of the Company tokens the account tools' Connection exchanges and
the stub's on-behalf-of check accepts. With poppy.auth.gateway.enabled the
gateway is the Company's issuer, poppy.auth.issuer, so that is the default,
and a poppy.account.subjectIssuer that names anything else fails the render:
nothing would serve that issuer, and every exchange would be refused.
Without the gateway: poppy.account.subjectIssuer, else poppy.auth.issuer.
Call with the root context.
*/}}
{{- define "omnia-demo.subjectIssuer" -}}
{{- $auth := .Values.poppy.auth -}}
{{- $set := .Values.poppy.account.subjectIssuer | default "" -}}
{{- if $auth.gateway.enabled -}}
{{- $gw := include "omnia-demo.gatewayIssuer" . -}}
{{- if and $set (ne $set $gw) -}}
{{- fail (printf "PoppySubjectIssuerNotGateway: poppy.account.subjectIssuer %q is not the gateway's issuer %q; with poppy.auth.gateway.enabled leave it empty (it defaults to the gateway) or set it to the gateway's issuer" $set $gw) -}}
{{- end -}}
{{- $gw -}}
{{- else -}}
{{- required "poppy.account.subjectIssuer or poppy.auth.issuer" (default $auth.issuer $set) -}}
{{- end -}}
{{- end -}}

{{/*
The account tools' Connection's tokenURL. With poppy.auth.gateway.enabled the
gateway's token endpoint, <gateway issuer>/oauth/token, is the default, and a
poppy.account.tokenURL that names anything else fails the render: the
exchange client is registered at the gateway alone. Without the gateway:
poppy.account.tokenURL, required. Call with the root context.
*/}}
{{- define "omnia-demo.accountTokenURL" -}}
{{- $set := .Values.poppy.account.tokenURL | default "" -}}
{{- if .Values.poppy.auth.gateway.enabled -}}
{{- $gw := printf "%s/oauth/token" (include "omnia-demo.gatewayIssuer" .) -}}
{{- if and $set (ne $set $gw) -}}
{{- fail (printf "PoppyTokenURLNotGateway: poppy.account.tokenURL %q is not the gateway's token endpoint %q; with poppy.auth.gateway.enabled leave it empty (it defaults to the gateway) or set it to the gateway's token endpoint" $set $gw) -}}
{{- end -}}
{{- $gw -}}
{{- else -}}
{{- required "poppy.account.tokenURL" $set -}}
{{- end -}}
{{- end -}}

{{/*
The account-route args of a stub (demos/mortgage-underwriting, main.go): the
Company's account routes, guarded by the check of the on-behalf-of token
(obo.go), when Company sign-in is on and this tenant is that Company. Renders
nothing otherwise. Call with (dict "root" $ "company" "larkmere"|"wrenmoor")
inside a container, at the container's indent.
*/}}
{{- define "omnia-demo.accountAPIArgs" -}}
{{- $p := .root.Values.poppy -}}
{{- if and $p.enabled $p.agent.enabled $p.auth.enabled (eq $p.agent.company .company) }}
args:
  - --account-api
  - {{ .company }}
  - --obo-issuer
  - {{ include "omnia-demo.subjectIssuer" .root | quote }}
  - --obo-audience
  - {{ required "poppy.account.audience" $p.account.audience | quote }}
{{- end }}
{{- end -}}

{{/*
Non-empty when privacy-api reaches the account tools' Key Vault key by keyless
workload identity: sign-in on, an azure-keyvault key with its vaultURL, and the
workspace identity's annotations to copy.
*/}}
{{- define "omnia-demo.privacyKMSIdentity" -}}
{{- $p := .Values.poppy | default dict -}}
{{- $enc := (($p.account | default dict).encryption | default dict) -}}
{{- if and $p.enabled ($p.auth | default dict).enabled (eq ($enc.kmsProvider | default "") "azure-keyvault") $enc.vaultURL .Values.workspace.runtimeIdentity.serviceAccountAnnotations .Values.workspace.privacy -}}
true
{{- end -}}
{{- end }}

{{/*
The host of one Company among personalAgent.knownCompanies, by the first label
of its name: call with (list . "larkmere"). Empty when none matches. The
assistant's pack and tool descriptions name the Companies by this host, so no
host of any one install is in the chart.
*/}}
{{- define "omnia-demo.companyHost" -}}
{{- $prefix := printf "%s." (index . 1) -}}
{{- range (index . 0).Values.personalAgent.knownCompanies -}}
{{- if hasPrefix $prefix . -}}{{- . -}}{{- end -}}
{{- end -}}
{{- end -}}
