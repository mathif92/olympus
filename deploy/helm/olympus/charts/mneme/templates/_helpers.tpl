{{- define "mneme.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "mneme.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: olympus
{{- end -}}

{{- define "mneme.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "mneme.postgresDsn" -}}
host={{ .Values.global.postgres.host | default (printf "%s-postgres" .Release.Name) }} port={{ .Values.global.postgres.port | default 5432 }} user={{ .Values.global.postgres.user | default "olympus" }} password={{ .Values.global.postgres.password | default "olympus_secret" }} dbname={{ .Values.dbname }} sslmode=disable
{{- end -}}

{{- define "mneme.themisUrl" -}}
{{- .Values.global.themis.url | default (printf "http://%s-themis:8091" .Release.Name) -}}
{{- end -}}

{{- define "mneme.saName" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name -}}
{{- end -}}