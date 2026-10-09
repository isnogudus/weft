{{/* Chart name, truncated to the 63 characters Kubernetes allows. */}}
{{- define "weft.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/* Fully qualified app name: release name, plus the chart name unless the release already contains it. */}}
{{- define "weft.fullname" -}}
{{- if .Values.fullnameOverride }}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- $name := default .Chart.Name .Values.nameOverride }}
{{- if contains $name .Release.Name }}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else }}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end }}
{{- end }}
{{- end }}

{{- define "weft.selectorLabels" -}}
app.kubernetes.io/name: {{ include "weft.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "weft.labels" -}}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{ include "weft.selectorLabels" . }}
app.kubernetes.io/version: {{ .Values.image.tag | default .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{- define "weft.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "weft.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/* True when a CA certificate is mounted from a Secret or ConfigMap. */}}
{{- define "weft.hasCaCert" -}}
{{- if or .Values.weft.caCert.secretName .Values.weft.caCert.configMapName }}true{{- end }}
{{- end }}
