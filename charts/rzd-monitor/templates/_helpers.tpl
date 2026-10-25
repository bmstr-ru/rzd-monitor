{{- define "rzd-monitor.fullname" -}}
{{- if contains .Chart.Name .Release.Name -}}
{{- .Release.Name | trunc 50 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-rzd-monitor" .Release.Name | trunc 50 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- define "rzd-monitor.selectorLabels" -}}
app.kubernetes.io/name: rzd-monitor
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}
{{- define "rzd-monitor.labels" -}}
{{ include "rzd-monitor.selectorLabels" . }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
helm.sh/chart: {{ printf "%s-%s" .Chart.Name .Chart.Version | quote }}
{{- end -}}
{{- define "rzd-monitor.image" -}}
{{ printf "%s:%s" .Values.image.repository .Values.image.tag | quote }}
{{- end -}}
{{- define "rzd-monitor.securityContext" -}}
allowPrivilegeEscalation: false
readOnlyRootFilesystem: true
capabilities:
  drop: ["ALL"]
{{- end -}}
