#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
config=$(OTEL_EXPORTER_OTLP_HEADERS='x-test=generic' OTEL_EXPORTER_OTLP_TRACES_HEADERS='x-test=traces' OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE='/test/ca.pem' OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE='/test/client.pem' OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY='/test/client.key' docker compose --profile '*' config --format json)
jq -e '
  [.services[] | select(.environment.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT != null)] as $services
  | ($services | length) > 0 and all($services[];
      .environment.OTEL_EXPORTER_OTLP_HEADERS == "x-test=generic" and
        .environment.OTEL_EXPORTER_OTLP_TRACES_HEADERS == "x-test=traces") and
      all(.services | to_entries[] | select(.key != "jaeger"); (.value.depends_on // {} | has("jaeger") | not)) and
      .services["api-gateway"].environment.OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE == "/test/ca.pem" and
      .services["api-gateway"].environment.OTEL_EXPORTER_OTLP_TRACES_CLIENT_CERTIFICATE == "/test/client.pem" and
      .services["api-gateway"].environment.OTEL_EXPORTER_OTLP_TRACES_CLIENT_KEY == "/test/client.key"
' <<<"$config" >/dev/null
disabled=$(OTEL_EXPORTER_OTLP_TRACES_ENDPOINT='' docker compose --profile '*' config --format json)
jq -e '
  [.services[] | select(.environment != null and (.environment | has("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")))] as $services
  | ($services | length) > 0 and all($services[]; .environment.OTEL_EXPORTER_OTLP_TRACES_ENDPOINT == "") and
    all(.services[] | select(.environment != null and (.environment | has("HOMENAVI_MQTT_TRACE_ENABLED"))); .environment.HOMENAVI_MQTT_TRACE_ENABLED == "auto")
' <<<"$disabled" >/dev/null
override=$(OTEL_EXPORTER_OTLP_TRACES_ENDPOINT='' HOMENAVI_MQTT_TRACE_ENABLED=true docker compose --profile '*' config --format json)
jq -e '
  all(.services[] | select(.environment != null and (.environment | has("HOMENAVI_MQTT_TRACE_ENABLED"))); .environment.HOMENAVI_MQTT_TRACE_ENABLED == "true")
' <<<"$override" >/dev/null
printf '%s\n' 'Compose tracing configuration and collector independence verified.'