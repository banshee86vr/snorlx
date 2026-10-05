{{/*
Expand the name of the chart.
*/}}
{{- define "snorlx.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
*/}}
{{- define "snorlx.fullname" -}}
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

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "snorlx.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "snorlx.labels" -}}
helm.sh/chart: {{ include "snorlx.chart" . }}
{{ include "snorlx.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "snorlx.selectorLabels" -}}
app.kubernetes.io/name: {{ include "snorlx.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Backend selector labels
*/}}
{{- define "snorlx.backend.selectorLabels" -}}
{{ include "snorlx.selectorLabels" . }}
app.kubernetes.io/component: backend
{{- end }}

{{/*
Frontend selector labels
*/}}
{{- define "snorlx.frontend.selectorLabels" -}}
{{ include "snorlx.selectorLabels" . }}
app.kubernetes.io/component: frontend
{{- end }}

{{/*
Database selector labels
*/}}
{{- define "snorlx.database.selectorLabels" -}}
{{ include "snorlx.selectorLabels" . }}
app.kubernetes.io/component: database
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "snorlx.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "snorlx.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Name of the release Secret
*/}}
{{- define "snorlx.secretName" -}}
{{ include "snorlx.fullname" . }}-secrets
{{- end }}

{{/*
Database URL. The password is injected from the Secret through $(POSTGRES_PASSWORD).
The actions trim surrounding whitespace: a leading newline makes pgx treat the value as a
keyword string and fall back to a local socket instead of the database Service.
*/}}
{{- define "snorlx.databaseUrl" -}}
{{- if .Values.database.internal -}}
postgresql://postgres:$(POSTGRES_PASSWORD)@{{ include "snorlx.fullname" . }}-db:5432/snorlx?sslmode=disable
{{- else -}}
{{- .Values.database.externalUrl -}}
{{- end -}}
{{- end }}

{{/*
Database image reference: digest when set, tag otherwise.
*/}}
{{- define "snorlx.databaseImage" -}}
{{- if .Values.database.image.digest }}
{{- printf "%s@%s" .Values.database.image.repository .Values.database.image.digest }}
{{- else }}
{{- printf "%s:%s" .Values.database.image.repository .Values.database.image.tag }}
{{- end }}
{{- end }}

{{/*
Public origin of the dashboard as seen by the browser. Explicit value, else https on the first
ingress host when ingress.tls is set. The backend rejects plain http on non-loopback hosts, so an
ingress without TLS needs an explicit backend.frontendUrl (for example when TLS terminates at a
cloud load balancer in front of the ingress). Without any of these, rendering stops.
*/}}
{{- define "snorlx.frontendUrl" -}}
{{- if .Values.backend.frontendUrl }}
{{- .Values.backend.frontendUrl | trimSuffix "/" }}
{{- else if and .Values.ingress.enabled .Values.ingress.hosts .Values.ingress.tls }}
{{- printf "https://%s" (index .Values.ingress.hosts 0).host }}
{{- else if .Values.ingress.enabled }}
{{- fail "ingress is enabled without ingress.tls: the backend rejects a plain http public origin. Set ingress.tls, or set backend.frontendUrl to the https origin served in front of the ingress" }}
{{- else }}
{{- fail "Set backend.frontendUrl to the public dashboard origin (for example https://snorlx.example.com, or http://localhost:8080 for port-forward access) or enable ingress with TLS" }}
{{- end }}
{{- end }}

{{/*
Backend URL used by the frontend nginx proxy for /api and /ws.
*/}}
{{- define "snorlx.backendUpstream" -}}
{{- printf "http://%s-backend:%v" (include "snorlx.fullname" .) .Values.service.backend.port }}
{{- end }}

{{/*
Reuse a value stored in the release Secret across upgrades, so generated secrets stay stable.
Usage: include "snorlx.stableSecret" (dict "ctx" . "key" "postgres-password" "value" .Values.database.password)
Returns the base64 encoded value.
*/}}
{{- define "snorlx.stableSecret" -}}
{{- $ctx := .ctx }}
{{- $existing := lookup "v1" "Secret" $ctx.Release.Namespace (include "snorlx.secretName" $ctx) }}
{{- if .value }}
{{- .value | b64enc }}
{{- else if and $existing $existing.data (index $existing.data .key) }}
{{- index $existing.data .key }}
{{- else }}
{{- randAlphaNum 48 | b64enc }}
{{- end }}
{{- end }}
