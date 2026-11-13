# Homenavi Deployment Guide

Homenavi supports a local Docker Compose evaluation, Docker Compose on a single trusted host, and Helm for Kubernetes. Start with local evaluation before connecting hardware or exposing a service beyond your network.

## Local evaluation

This is the recommended first run. It uses bundled PostgreSQL, Valkey, Mosquitto, SeaweedFS, Jaeger, and development-only credentials. It does not need a domain name, TLS certificate, cloud account, or physical controller.

1. Install Docker Engine with Docker Compose v2, then run:

   ```sh
   git clone https://github.com/PetoAdam/homenavi.git
   cd homenavi
   cp .env.example .env
   docker compose pull
   docker compose up -d
   docker compose ps
   ```

2. Open `http://localhost` and create the first account. If port 80 is already in use, set `NGINX_HOST_PORT=8088` in `.env` and open `http://localhost:8088`.

3. Stop the evaluation with:

   ```sh
   docker compose down --volumes --remove-orphans
   ```

   The checkout also contains bind-mounted development state such as `postgres-data/`; delete the disposable checkout when you want a completely clean evaluation.

Released Homenavi service images are published to GitHub Container Registry as `ghcr.io/petoadam/homenavi-<service>:<tag>`. Compose uses these images by default. Set `HN_VERSION=vX.Y.Z` before `docker compose pull` to evaluate a specific release; `latest` follows the current release. Use `docker compose up --build` only when testing local source changes.

Zigbee2MQTT is deliberately excluded from a local evaluation. To use a real coordinator, set `ZIGBEE2MQTT_DEVICE_MAPPING`, `ZIGBEE2MQTT_CONFIG_SERIAL_PORT`, and the other `ZIGBEE2MQTT_CONFIG_*` settings in `.env`, then start the hardware profile with `docker compose --profile hardware up -d`.

## Choose a mode

Use Docker Compose for a single machine or small trusted homelab. It provisions bundled dependencies and is the normal path when integrations are managed with Docker Compose.

Use Helm for Kubernetes or GitOps. It uses Kubernetes Services, persistent volumes, and Helm-native integration management. When using Argo CD or Flux, the Git repository is the desired-state source; do not use in-app integration lifecycle actions as the source of truth.

## Docker Compose On A Trusted Host

1. Clone the repository and create a deployment environment file:

   ```sh
   git clone https://github.com/PetoAdam/homenavi.git
   cd homenavi
   cp .env.example .env
   ```

2. Set a release tag and replace all development defaults in `.env`:

   ```dotenv
   HN_VERSION=vX.Y.Z
   NGINX_MODE=prod
   PROFILE_PICTURE_STORAGE_S3_ACCESS_KEY=<generated-secret>
   PROFILE_PICTURE_STORAGE_S3_SECRET_KEY=<generated-secret>
   POSTGRES_PASSWORD=<generated-secret>
   ```

   Set the Google and SMTP variables only when those features are enabled. Mount the TLS certificate and key through `NGINX_CERT_ORIGIN` and `NGINX_KEY_ORIGIN`.

3. Start the stack and check its status:

   ```sh
   docker compose pull
   docker compose up -d
   docker compose ps
   ```

4. Make the public DNS name resolve to the Docker host. Before internet exposure, use a Compose override or host firewall to restrict the development ports exposed by the base Compose file; only the TLS edge proxy should be publicly reachable. Do not publish internal service, database, broker, or observability ports on the public network.

Compose uses `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT` as a single shared collector setting. When unset, it points to bundled Jaeger; set it to an empty value to disable trace export. MQTT trace propagation defaults to `auto`, following that endpoint; set `HOMENAVI_MQTT_TRACE_ENABLED=true` or `false` to override it. Updated frontend images check the gateway's runtime tracing configuration without delaying rendering, so early requests may not be traced; older prebuilt images must be updated to honor the runtime opt-out. Core services do not depend on Jaeger starting, and collector or exporter failures can drop traces without preventing normal requests. Trace-correlation cache operations are time-bounded and may be skipped when unavailable. These guarantees do not remove the application's other database, broker, authentication, or session-store dependencies.

OAuth and MCP derive their public URLs from the host and scheme sent through the edge proxy, so a normal deployment has no public-URL setting. Use the individual endpoint variables only for a split-domain deployment.

Shared automation live events are queued for best-effort delivery without blocking workflow execution. Events can be dropped when Redis is unavailable, the queue is full, or a pod stops; clients can still retrieve persisted run state.

Command results are not durable across every database outage or process restart: the MQTT client acknowledges messages independently of database persistence, and its default session is not persistent. A command without a recorded result expires with an unknown outcome and may require reconciliation against the device; it is not proof that the command failed.

HTTP request metrics use route templates as the `endpoint` label and `unmatched` for requests without a matched route. Nonstandard methods use the `_OTHER` label; dashboards and alerts using concrete paths should be updated accordingly.

Mosquitto is the default MQTT 5 broker and Valkey provides Redis-protocol cache/session storage. To use the explicit, single-node EMQX compatibility profile, run `docker compose -f docker-compose.yml -f docker-compose.emqx.yml up -d`; do not use that profile for clustered production without a separate EMQX license review.

Mosquitto generates its runtime configuration from the `MOSQUITTO_*` values in `.env`. Broker-state persistence is disabled by default for a simpler local setup; set `MOSQUITTO_PERSISTENCE=true` to retain broker state in the `mosquitto_data` named volume. Use `MOSQUITTO_EXTRA_CONFIG` only for newline-separated native directives not represented by the documented settings. Zigbee2MQTT accepts the documented `ZIGBEE2MQTT_CONFIG_*` variables directly, including `ZIGBEE2MQTT_DEVICE_MAPPING` and `ZIGBEE2MQTT_CONFIG_SERIAL_PORT` for a USB coordinator, and persists its working data in `zigbee2mqtt-data`; no default broker or Zigbee2MQTT configuration file is required.

## Helm and GitOps

1. Create Kubernetes secrets for non-public settings. Do not put passwords, OAuth credentials, or private keys in Helm values or Git:

   ```sh
   kubectl -n homenavi create secret generic homenavi-runtime-env \
     --from-env-file=./runtime.env
   ```

2. Put shared URLs and observability in one values file:

```yaml
global: { otelExporterOtlpTracesEndpoint: http://jaeger.monitoring.svc.cluster.local:4318/v1/traces, envFromSecrets: [homenavi-runtime-env] }
ingress: { enabled: true, className: nginx, hosts: [{ host: home.example.com, paths: [{ path: /, pathType: Prefix, service: edge-proxy, port: 80 }] }] }
```

   The primary `ingress.hosts[].host` is the only public-host setting. The edge proxy forwards the public host and scheme, which OAuth and MCP use to derive absolute protocol URLs. Per-service environment values are still supported when an endpoint must differ. `global.otelExporterOtlpTracesEndpoint` is injected into all OTEL-capable core services. The same env file can provide `MOSQUITTO_*` and `ZIGBEE2MQTT_CONFIG_*`: the Mosquitto command reads those values to generate its runtime configuration, and Zigbee2MQTT reads its supported environment overrides directly. Set `MOSQUITTO_PERSISTENCE=true` when the Helm claim should retain broker state.

3. Validate before applying:

   ```sh
   helm lint ./helm/homenavi
   helm template homenavi ./helm/homenavi -f values.production.yaml > /tmp/homenavi.yaml
   helm upgrade --install homenavi ./helm/homenavi -n homenavi --create-namespace -f values.production.yaml
   ```

For Argo CD, use the same values under `spec.source.helm.values` and pin the chart and `global.imageTag` to the same release. Set `services.frontend.image.tag` to that release only when an explicit frontend override is needed.

Helm uses readiness-gated rolling updates for automation-service. During the transition from the older command-result protocol, mixed versions sharing PostgreSQL and MQTT can process each other's in-flight results incorrectly. Drain active workflows and stop old replicas before starting the new version when that risk is unacceptable; interrupted work may require reconciliation.

## Quick-Start Standard

Homenavi's local route is intentionally shorter than a durable deployment: clone, copy `.env.example`, pull, run, and open the browser. Homenavi should keep physical hardware, public TLS, external credentials, and Kubernetes out of the first-run path, then introduce them only when the user chooses the corresponding deployment mode.

The boundary is deliberate: local evaluation uses public development credentials only inside its bundled network; any durable or exposed deployment must replace them and restrict network exposure.

## Operations checklist

- Back up PostgreSQL, persistent Zigbee2MQTT data, integration configuration, object storage, and Mosquitto data when `MOSQUITTO_PERSISTENCE=true`.
- Pin releases with `HN_VERSION` in Compose or matching chart/image versions in Helm; avoid an unreviewed `latest` deployment.
- Use a real TLS certificate and redirect HTTP to HTTPS.
- Ensure the public edge proxy forwards `Host` and `X-Forwarded-Proto`; OAuth and MCP metadata depend on the public origin.
- Keep the OTLP collector reachable from every service when tracing is enabled; leave the endpoint blank to disable exporter injection in Helm.
- For external MQTT, PostgreSQL, Redis, or object storage, use the existing chart values and secret references documented in [Helm HA Operations](helm_ha_operations.md).

See [Local Build Guide](local_build.md) for developer workflows, [Nginx Guide](nginx_guide.md) for TLS and reverse-proxy behavior, and [MCP](mcp.md) for OAuth scopes and MCP operations.