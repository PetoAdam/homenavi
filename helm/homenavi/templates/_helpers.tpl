{{- define "homenavi.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "homenavi.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- $name := include "homenavi.name" . -}}
{{- if contains $name .Release.Name -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- printf "%s-%s" .Release.Name $name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "homenavi.labels" -}}
helm.sh/chart: {{ include "homenavi.chart" . }}
app.kubernetes.io/name: {{ include "homenavi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end -}}

{{- define "homenavi.selectorLabels" -}}
app.kubernetes.io/name: {{ include "homenavi.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end -}}

{{- define "homenavi.componentName" -}}
{{- printf "%s-%s" (include "homenavi.fullname" .root) .component | trunc 63 | trimSuffix "-" -}}
{{- end -}}

{{- define "homenavi.componentLabels" -}}
{{ include "homenavi.labels" .root }}
app.kubernetes.io/component: {{ .component }}
{{- end -}}

{{- define "homenavi.serviceAccountName" -}}
{{- if .Values.serviceAccount.create -}}
{{- default (include "homenavi.fullname" .) .Values.serviceAccount.name -}}
{{- else -}}
{{- default "default" .Values.serviceAccount.name -}}
{{- end -}}
{{- end -}}

{{- define "homenavi.image" -}}
{{- $repository := required "service.image.repository is required" .service.image.repository -}}
{{- $tag := default .root.Values.global.imageTag .service.image.tag -}}
{{- printf "%s:%s" $repository $tag -}}
{{- end -}}

{{- define "homenavi.jwtSecretName" -}}
{{- default "homenavi-jwt" .Values.jwt.existingSecretName -}}
{{- end -}}

{{- define "homenavi.jwtStaticSecret" -}}
{{- and .Values.jwt.createSecret (not .Values.jwt.existingSecretName) (ne (trim .Values.jwt.publicKey) "") (ne (trim .Values.jwt.privateKey) "") -}}
{{- end -}}

{{- define "homenavi.jwtBootstrapEnabled" -}}
{{- and .Values.jwt.createSecret (not .Values.jwt.existingSecretName) .Values.jwt.bootstrap.enabled (not (eq (include "homenavi.jwtStaticSecret" .) "true")) -}}
{{- end -}}
