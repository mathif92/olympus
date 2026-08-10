{{- define "olympus.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{- define "olympus.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" }}
{{- else -}}
{{- $name := default .Chart.Name .Values.nameOverride -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" }}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" }}
{{- end -}}
{{- end -}}
{{- end }}

{{- define "olympus.labels" -}}
app.kubernetes.io/name: {{ include "olympus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" }}
{{- end }}

{{- define "olympus.selectorLabels" -}}
app.kubernetes.io/name: {{ include "olympus.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{- define "olympus.storageBackend" -}}
{{- if .Values.storageBackend -}}
{{- .Values.storageBackend }}
{{- else if .Values.minio.enabled -}}
minio
{{- else -}}
hybrid
{{- end -}}
{{- end }}

{{- define "olympus.minioServerList" -}}
{{- $sts := printf "%s-minio" (include "olympus.fullname" .) }}
{{- $head := printf "%s-minio" (include "olympus.fullname" .) }}
{{- $ns := .Release.Namespace }}
{{- $out := "" }}
{{- range $i := until (.Values.minio.distributedNodes | int) }}
{{- $out = printf "%s %s-%d.%s.%s.svc:9000/data" $out $sts $i $head $ns }}
{{- end }}
{{- trim $out }}
{{- end }}