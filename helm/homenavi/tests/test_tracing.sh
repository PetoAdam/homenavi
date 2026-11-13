#!/usr/bin/env sh
set -eu

chart_dir=${1:-helm/homenavi}
rendered=$(mktemp)
trap 'rm -f "$rendered"' EXIT

helm template homenavi "$chart_dir" --set services.mock-adapter.enabled=true --set services.zigbee-adapter.enabled=true -s templates/deployments.yaml >"$rendered"
if ! awk -v RS='---' 'index($0, "- name: automation-service\n") && index($0, "type: RollingUpdate") && index($0, "maxUnavailable: 0") && index($0, "maxSurge: 1") && index($0, "readinessProbe:") && index($0, "path: /health") { found=1 } END { exit !found }' "$rendered"; then
  echo "automation-service must use a readiness-gated rolling update" >&2
  exit 1
fi
for service in api-gateway auth-service user-service dashboard-service email-service profile-picture-service device-hub integration-proxy mock-adapter zigbee-adapter automation-service entity-registry-service history-service mcp-service weather-service; do
  if ! awk -v RS='---' -v service="$service" 'index($0, "- name: " service "\n") && index($0, "- name: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") { found=1 } END { exit !found }' "$rendered"; then
    echo "missing OTLP endpoint for $service" >&2
    exit 1
  fi
done
for service in device-hub mock-adapter zigbee-adapter automation-service entity-registry-service; do
  if ! awk -v RS='---' -v service="$service" 'index($0, "- name: " service "\n") && index($0, "- name: HOMENAVI_MQTT_TRACE_ENABLED") { found=1 } END { exit !found }' "$rendered"; then
    echo "missing MQTT propagation setting for $service" >&2
    exit 1
  fi
done
if ! grep -q 'value: "http://otel-collector:4318/v1/traces"' "$rendered" || ! grep -q 'name: HOMENAVI_BROWSER_OTEL_ENABLED' "$rendered"; then
  echo "missing default collector endpoint or browser ingestion" >&2
  exit 1
fi
if grep -qE '^[[:space:]]*- name: (jaeger|grafana)$' "$rendered"; then
  echo "Jaeger and Grafana must be disabled by default" >&2
  exit 1
fi
helm template homenavi "$chart_dir" --set global.mqttTracePropagationEnabled=false --set global.otelExporterOtlpTracesEndpoint=http://collector:4318/v1/traces --set services.jaeger.enabled=true >"$rendered"
if grep -q 'name: HOMENAVI_MQTT_TRACE_ENABLED' "$rendered" || ! grep -q 'value: "http://collector:4318/v1/traces"' "$rendered" || ! grep -q 'name: otlp-http' "$rendered"; then
  echo "tracing overrides or bundled Jaeger OTLP service are incorrect" >&2
  exit 1
fi
helm template homenavi "$chart_dir" --set-string global.otelExporterOtlpTracesEndpoint= --set-string services.api-gateway.env.HOMENAVI_BROWSER_OTEL_ENABLED=false -s templates/deployments.yaml >"$rendered"
if grep -q 'name: OTEL_EXPORTER_OTLP_TRACES_ENDPOINT' "$rendered" || ! awk -v RS='---' 'index($0, "- name: api-gateway\n") && index($0, "name: HOMENAVI_BROWSER_OTEL_ENABLED\n") && index($0, "value: \"false\"") { found=1 } END { exit !found }' "$rendered"; then
  echo "tracing export or browser ingestion opt-out is incorrect" >&2
  exit 1
fi