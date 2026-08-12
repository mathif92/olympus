{{- define "amphora.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "amphora.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: olympus
{{- end -}}

{{- define "amphora.selectorLabels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "amphora.postgresDsn" -}}
host={{ .Values.global.postgres.host | default (printf "%s-postgres" .Release.Name) }} port={{ .Values.global.postgres.port | default 5432 }} user={{ .Values.global.postgres.user | default "olympus" }} password={{ .Values.global.postgres.password | default "olympus_secret" }} dbname={{ .Values.dbname }} sslmode=disable
{{- end -}}

{{- define "amphora.themisUrl" -}}
{{- .Values.global.themis.url | default (printf "http://%s-themis:8091" .Release.Name) -}}
{{- end -}}

{{- define "amphora.minioEndpoint" -}}
{{- .Values.global.minio.endpoint | default (printf "http://%s-minio:%v" .Release.Name (.Values.global.minio.port | default 9000)) -}}
{{- end -}}

{{- define "amphora.redisUrl" -}}
{{- printf "redis://%s-redis:6379" .Release.Name -}}
{{- end -}}

{{- define "amphora.saName" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name -}}
{{- end -}}