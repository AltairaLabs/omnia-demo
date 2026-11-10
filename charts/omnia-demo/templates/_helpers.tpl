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
