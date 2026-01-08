{{/*
Expand the name of the chart.
*/}}
{{- define "bright.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create a default fully qualified app name.
We truncate at 63 chars because some Kubernetes name fields are limited to this (by the DNS naming spec).
If release name contains chart name it will be used as a full name.
*/}}
{{- define "bright.fullname" -}}
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
{{- define "bright.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "bright.labels" -}}
helm.sh/chart: {{ include "bright.chart" . }}
{{ include "bright.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "bright.selectorLabels" -}}
app.kubernetes.io/name: {{ include "bright.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Create the name of the service account to use
*/}}
{{- define "bright.serviceAccountName" -}}
{{- if .Values.serviceAccount.create }}
{{- default (include "bright.fullname" .) .Values.serviceAccount.name }}
{{- else }}
{{- default "default" .Values.serviceAccount.name }}
{{- end }}
{{- end }}

{{/*
Image name
*/}}
{{- define "bright.image" -}}
{{- $registry := .Values.image.registry -}}
{{- $repository := .Values.image.repository -}}
{{- $tag := .Values.image.tag | default .Chart.AppVersion -}}
{{- printf "%s/%s:%s" $registry $repository $tag -}}
{{- end }}

{{/*
PVC name
*/}}
{{- define "bright.pvcName" -}}
{{- if .Values.persistence.existingClaim }}
{{- .Values.persistence.existingClaim }}
{{- else }}
{{- include "bright.fullname" . }}-data
{{- end }}
{{- end }}

{{/*
Validate masterKey configuration - ensure only one method is used
*/}}
{{- define "bright.validateMasterKeyConfig" -}}
{{- if and .Values.config.masterKey .Values.config.masterKeySecret.enabled }}
{{- fail "ERROR: Cannot use both config.masterKey and config.masterKeySecret.enabled simultaneously. Please use only one method: (1) Direct value: Set config.masterKey and leave config.masterKeySecret.enabled=false, OR (2) Secret reference: Set config.masterKeySecret.enabled=true and provide config.masterKeySecret.name" }}
{{- end }}
{{- end }}

{{/*
Generate Raft peers list for StatefulSet
Creates comma-separated list of peer addresses like: bright-0.bright-headless:7000,bright-1.bright-headless:7000
*/}}
{{- define "bright.raftPeers" -}}
{{- $fullname := include "bright.fullname" . -}}
{{- $namespace := .Release.Namespace -}}
{{- $raftPort := int .Values.raft.port -}}
{{- $replicaCount := int .Values.replicaCount -}}
{{- $peers := list -}}
{{- range $i := until $replicaCount -}}
{{- $peers = append $peers (printf "%s-%d.%s-headless.%s.svc.cluster.local:%d" $fullname $i $fullname $namespace $raftPort) -}}
{{- end -}}
{{- join "," $peers -}}
{{- end }}
