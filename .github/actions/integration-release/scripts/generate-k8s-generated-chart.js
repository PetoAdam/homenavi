const fs = require('fs');
const path = require('path');

const tag = process.env.TAG || '';
const imageName = process.env.IMAGE_NAME || '';
const imageRef = process.env.IMAGE_REF || '';
const metadataPath = process.env.METADATA_PATH || '';
const manifestPath = process.env.MANIFEST_PATH || '';
const outputDir = process.env.OUTPUT_DIR || '';

if (!tag) throw new Error('TAG is required');
if (!imageName) throw new Error('IMAGE_NAME is required');
if (!imageRef) throw new Error('IMAGE_REF is required');
if (!metadataPath) throw new Error('METADATA_PATH is required');
if (!manifestPath) throw new Error('MANIFEST_PATH is required');
if (!outputDir) throw new Error('OUTPUT_DIR is required');

const readJson = (filePath) => JSON.parse(fs.readFileSync(filePath, 'utf8'));
const stripLeadingV = (value) => String(value || '').replace(/^v(?=\d)/, '');
const sanitizeName = (value, fallback) => {
  const normalized = String(value || '')
    .toLowerCase()
    .replace(/[^a-z0-9-]+/g, '-')
    .replace(/^-+|-+$/g, '')
    .slice(0, 63);
  return normalized || fallback;
};
const writeFile = (relativePath, content) => {
  const target = path.join(outputDir, relativePath);
  fs.mkdirSync(path.dirname(target), { recursive: true });
  fs.writeFileSync(target, content);
};

const metadata = readJson(metadataPath);
const manifest = readJson(manifestPath);
const chartVersion = stripLeadingV(tag);
const appVersion = String(tag);
const displayName = metadata.name || manifest.name || imageName;
const appName = sanitizeName(metadata.id || manifest.id || imageName, 'integration');

writeFile('Chart.yaml', `apiVersion: v2
name: ${imageName}
description: Generated Kubernetes chart for ${displayName}
type: application
version: ${chartVersion}
appVersion: ${JSON.stringify(appVersion)}
`);

writeFile('values.yaml', `replicaCount: 1

image:
  repository: ${imageRef.substring(0, imageRef.lastIndexOf(':'))}
  tag: ${JSON.stringify(appVersion)}
  pullPolicy: IfNotPresent

service:
  type: ClusterIP
  port: 8099

probes:
  path: /healthz

resources: {}
extraEnv: []
extraVolumeMounts: []
extraVolumes: []
podLabels: {}
`);

writeFile('templates/_helpers.tpl', `{{- define "integration-chart.name" -}}
${appName}
{{- end -}}

{{- define "integration-chart.fullname" -}}
{{- if .Values.fullnameOverride -}}
{{- .Values.fullnameOverride | trunc 63 | trimSuffix "-" -}}
{{- else -}}
{{- .Release.Name | trunc 63 | trimSuffix "-" -}}
{{- end -}}
{{- end -}}
`);

writeFile('templates/deployment.yaml', `apiVersion: apps/v1
kind: Deployment
metadata:
  name: {{ include "integration-chart.fullname" . }}
  labels:
    app.kubernetes.io/name: {{ include "integration-chart.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
spec:
  replicas: {{ .Values.replicaCount }}
  selector:
    matchLabels:
      app.kubernetes.io/name: {{ include "integration-chart.name" . }}
      app.kubernetes.io/instance: {{ .Release.Name }}
  template:
    metadata:
      labels:
        app.kubernetes.io/name: {{ include "integration-chart.name" . }}
        app.kubernetes.io/instance: {{ .Release.Name }}
{{- with .Values.podLabels }}
{{ toYaml . | indent 8 }}
{{- end }}
    spec:
{{- with .Values.extraVolumes }}
      volumes:
{{ toYaml . | indent 8 }}
{{- end }}
      containers:
        - name: ${appName}
          image: "{{ .Values.image.repository }}:{{ .Values.image.tag }}"
          imagePullPolicy: {{ .Values.image.pullPolicy }}
          env:
            - name: PORT
              value: "8099"
{{- with .Values.extraEnv }}
{{ toYaml . | indent 12 }}
{{- end }}
          ports:
            - name: http
              containerPort: 8099
              protocol: TCP
          readinessProbe:
            httpGet:
              path: {{ .Values.probes.path }}
              port: http
          livenessProbe:
            httpGet:
              path: {{ .Values.probes.path }}
              port: http
{{- with .Values.resources }}
          resources:
{{ toYaml . | indent 12 }}
{{- end }}
{{- with .Values.extraVolumeMounts }}
          volumeMounts:
{{ toYaml . | indent 12 }}
{{- end }}
`);

writeFile('templates/service.yaml', `apiVersion: v1
kind: Service
metadata:
  name: {{ include "integration-chart.fullname" . }}
  labels:
    app.kubernetes.io/name: {{ include "integration-chart.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
spec:
  type: {{ .Values.service.type }}
  ports:
    - name: http
      port: {{ .Values.service.port }}
      targetPort: http
      protocol: TCP
  selector:
    app.kubernetes.io/name: {{ include "integration-chart.name" . }}
    app.kubernetes.io/instance: {{ .Release.Name }}
`);