{{/*
Umbrella-level helpers. Subcharts define their own helpers (Helm scopes
templates to the chart that declares them), so the umbrella mostly wires
global values into subchart values and provides helpers for the bundled
PostgreSQL and MinIO components that live in `templates/`.
*/}}

{{- define "olympus.labels" -}}
app.kubernetes.io/name: olympus
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/part-of: olympus
helm.sh/chart: {{ .Chart.Name }}-{{ .Chart.Version | replace "+" "_" }}
{{- end -}}

{{- define "olympus.postgres.dsn" -}}
{{- $g := .Values.global -}}
host={{ ternary $g.postgres.host .Values.postgres.host (empty $g.postgres.host) }} port={{ ternary $g.postgres.port .Values.postgres.port (empty $g.postgres.port) }} user={{ $g.postgres.user | default .Values.postgres.user }} password={{ $g.postgres.password | default .Values.postgres.password }} dbname={{ .dbname }} sslmode=disable
{{- end -}}
