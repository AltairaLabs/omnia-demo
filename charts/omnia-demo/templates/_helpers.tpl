{{/*
The Provider an agent uses: llm.providerRef when it is set, so every demo runs
on one Provider that already exists in the namespace (a keyless Azure AI
Foundry Provider, say). Otherwise the demo's own Provider, named by `own`.
Call with (dict "root" $ "own" "<demo>-llm").
*/}}
{{- define "omnia-demo.llmProvider" -}}
{{- default .own .root.Values.llm.providerRef -}}
{{- end -}}

{{/*
An image reference. An empty tag means the chart's appVersion: the publish
workflow tags every image with the release version, so a chart release pulls
the images built with it.
Call with (dict "root" $ "image" .Values.<demo>.image).
*/}}
{{- define "omnia-demo.image" -}}
{{- .image.repository }}:{{ default .root.Chart.AppVersion .image.tag -}}
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
