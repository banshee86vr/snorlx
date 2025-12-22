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
Database URL
*/}}
{{- define "snorlx.databaseUrl" -}}
{{- if .Values.database.internal }}
postgresql://postgres:$(POSTGRES_PASSWORD)@{{ include "snorlx.fullname" . }}-db:5432/snorlx?sslmode=disable
{{- else }}
{{- .Values.database.externalUrl }}
{{- end }}
{{- end }}

