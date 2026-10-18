{{- define "concession.fullname" -}}
{{- printf "%s-%s" .Release.Name .Chart.Name | trunc 53 | trimSuffix "-" -}}
{{- end -}}

{{- define "concession.labels" -}}
app.kubernetes.io/name: {{ .Chart.Name }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end -}}

{{- define "concession.secretName" -}}
{{- if .Values.existingSecret -}}{{ .Values.existingSecret }}{{- else -}}{{ include "concession.fullname" . }}{{- end -}}
{{- end -}}

{{- define "concession.scheme" -}}
{{- if .Values.tls -}}https{{- else -}}http{{- end -}}
{{- end -}}
