<p align="center">
	<img src="frontend/public/icons/icon-192x192.png" alt="Homenavi" width="72" height="72" />
</p>

<h1 align="center" style="margin-bottom: 0;">
	<span style="font-family: 'Manrope', 'Montserrat', 'Inter', 'Segoe UI', Arial, sans-serif; letter-spacing: 0.18em; text-transform: uppercase;">Homenavi</span>
</h1>

<p align="center"><strong>A smart home platform for developers, by developers. Modern, microservice-based, and built to be extended.</strong></p>

<p align="center">
	<a href="#quickstart">Quickstart</a> •
	<a href="doc/architecture_diagram.md">Architecture</a> •
	<a href="https://github.com/PetoAdam/homenavi/issues">Issues</a>
</p>

<p align="center">
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml"><img alt="Build Frontend Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml"><img alt="Build User Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml"><img alt="Build API Gateway Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml"><img alt="Build Auth Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/device_hub_docker_build.yaml"><img alt="Build Device Hub Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/device_hub_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/email_service_docker_build.yaml"><img alt="Build Email Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/email_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/profile_picture_service_docker_build.yaml"><img alt="Build Profile Picture Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/profile_picture_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/echo_service_docker_build.yaml"><img alt="Build Echo Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/echo_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/history_service_docker_build.yaml"><img alt="Build History Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/history_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/zigbee_adapter_docker_build.yaml"><img alt="Build Zigbee Adapter Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/zigbee_adapter_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/thread_adapter_docker_build.yaml"><img alt="Build Thread Adapter Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/thread_adapter_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/automation_service_docker_build.yaml"><img alt="Build Automation Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/automation_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/entity_registry_service_docker_build.yaml"><img alt="Build Entity Registry Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/entity_registry_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/weather_service_docker_build.yaml"><img alt="Build Weather Service Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/weather_service_docker_build.yaml/badge.svg" /></a>
	<a href="https://github.com/PetoAdam/homenavi/actions/workflows/integration_proxy_docker_build.yaml"><img alt="Build Integration Proxy Docker Image" src="https://github.com/PetoAdam/homenavi/actions/workflows/integration_proxy_docker_build.yaml/badge.svg" /></a>
</p>

Welcome to Homenavi — your open, hackable smart home solution. Built with a modern microservices architecture, Homenavi is designed for tinkerers, makers, and pros who want full control and easy extensibility.

---


## Table of Contents
1. 🚀 Why Homenavi
2. 🧩 Architecture Overview
3. 🔮 Smart Home Vision (Current + Upcoming)
4. 🐳 Quickstart
5. 📂 Service Directory
6. 🔒 Security & Auth
7. 📊 Observability
8. ⚡ WebSockets & Real‑Time
9. 🔌 Extending the Platform
10. 🗺️ Roadmap
11. ⚙️ Configuration & Environment
12. 📦 CI/CD
13. 🤝 Contributing & Community
14. ❓ FAQ
15. 📜 License

---

## 1. 🚀 Why Homenavi
- **Microservice-first:** Each core feature is its own service – scale, swap, or extend as you like.
- **Modern stack:** Go, React, Python, Docker, and more.
- **Dev-friendly:** Easy to run, hack, and contribute.
- **Open & Transparent:** 100% open source, MIT licensed.
- **Cloud or Home:** Run it on your Raspberry Pi, your server, or in the cloud.
- **Observability built-in:** Prometheus metrics, Jaeger tracing, and request/correlation IDs for easy debugging and monitoring.
- **WebSocket support:** Real-time communication with cookie-based JWT authentication.
- **Extensible by design:** Add new device protocols, automations, and integrations with minimal friction.

---

## 2. 🧩 Architecture Overview

Further reading:
* [ERS / HDP / Device Hub — How it works (current)](doc/ers_hdp_devicehub_overview.md)
* [Architecture diagram (current)](doc/architecture_diagram.md)

Current Core:
* API Gateway (Go): Routing, JWT verification, rate limit, WebSocket upgrade.
* Device Hub (Go): Central device inventory and **HDP-only** adapter bridge over MQTT.
* History Service (Go): Persists HDP device state into Postgres and serves query endpoints for charts.
* Automation Service (Go): Automation/workflow orchestration (graph builder UI backend).
* Weather Service (Go): Cached weather API used by the frontend via the gateway.
* Auth Service (Go): Login, password management, 2FA (email now, TOTP coming), lockout logic.
* User Service (Go): Profile, roles, administrative user actions.
* Dashboard Service (Go): Stores per-user dashboards and widget configuration.
* Marketplace API (Go, external): Integration catalog + downloads/trending stats (consumed by frontend and integration-proxy).
* Entity Registry Service (Go): Home inventory primitives (rooms/tags/devices) and discovery surface.
* Email Service (Go): Outbound verification & notification emails.
* Profile Picture Service (Python): Image handling (avatars, basic processing).
* Echo Service (Python): Real-time WebSocket demo & test surface.
* Frontend (React + Vite + PWA): Auth flows, user management, device UI, dashboards.
* Adapters:
	* Zigbee Adapter (Go): Zigbee2MQTT bridge that emits HDP state/metadata/events and consumes HDP commands.
	* Thread Adapter (Go): Placeholder implementation that emits HDP hello/status/pairing and acks commands.
* Infrastructure: PostgreSQL, Redis, Nginx, Prometheus, Jaeger, (Grafana ready).

Key Design Principles:
* Clear separation of auth vs user domain.
* Stateless services (use Redis / DB for state persistence & coordination).
* Consistent JSON error schema across services.
* Incremental addition of domain (devices, automations, adapters) without core rewrites.
* SPA frontend with history fallback (direct links work out of the box).

Current Features (Implemented):
* **Device abstraction via HDP v1:** adapters translate protocol payloads into a single internal contract consumed by core services. See `doc/hdp.md`.
* **Adapters (today):** Zigbee2MQTT → HDP bridge (plus pairing/commands); Thread adapter placeholder using the same HDP surfaces.
* **ERS + Device Hub boundary:** ERS owns names/rooms/tags/map metadata; device-hub owns realtime telemetry, pairing, commands. See `doc/ers_hdp_devicehub_overview.md`.
* **Customizable UI dashboards:** widget-based Home dashboard with Edit mode + per-user persistence via Dashboard Service. See `doc/dashboard_ui_functional_spec.md`.
* **Integration marketplace flow:** frontend queries the Marketplace API directly for catalog + stats, posts download increments on successful installs, and integration-proxy installs using runtime-resolved `deployment_artifacts` from marketplace metadata.
* **Automation engine + scheduling:** rule/workflow engine with manual triggers and **cron schedule triggers**, plus run tracking and live run stream via websocket. (APIs documented in `doc/external_api_surface.md`.)

---

## 3. 🔮 Smart Home Vision (Current + Upcoming)
Homenavi’s “vision” is already partially implemented (dashboards, HDP-based adapters, automations). The next steps are about **expanding capabilities** and **hardening extensibility**:

Upcoming focus areas:
* **More adapters / protocol coverage:** Matter, Z-Wave, BLE, and cloud integrations.
* **Automation evolution:** versioned deployments, richer trigger/action/condition library, improved editor UX, and safe integration-provided automation steps.
* **Smarter scheduling:** beyond cron (sunrise/sunset and other home-aware schedules).
* **Scene & Mode Management:** grouped device state snapshots and home modes (Away / Night / Eco).
* **Presence & Energy modules:** occupancy inference; energy usage aggregation.
* **Integration marketplace model:** verified/unverified integrations, secure sandboxing for third-party widgets.
* **Edge nodes:** lightweight agents pushing device state/events to the core cluster.

Contributions and feedback on these modules are welcome!

---

<a id="quickstart"></a>
## 4. 🐳 Quickstart

```sh
git clone https://github.com/PetoAdam/homenavi.git
cd homenavi
cp .env.example .env   # adjust secrets / paths
docker compose up --build
```

Entry Points:
* Frontend: http://localhost (served via Nginx)
* API Gateway (REST): http://localhost/api
* Prometheus: http://localhost:9090
* Jaeger UI: http://localhost:16686
* (Grafana optional) http://localhost:3000

See `doc/local_build.md` and `doc/nginx_guide.md` for deeper setup details.

---


## 5. 📂 Service Directory
| Service | Path | Purpose | Docker Build Tag |
|---------|------|---------|------------------|
| API Gateway | `api-gateway/` | Request routing, auth verification, rate limiting, WS proxy | `homenavi-api-gateway:latest` |
| Auth Service | `auth-service/` | Credentials, tokens, 2FA, lockout logic | `homenavi-auth-service:latest` |
| User Service | `user-service/` | User profiles, roles, admin operations | `homenavi-user-service:latest` |
| Dashboard Service | `dashboard-service/` | Per-user dashboards + widget persistence | `homenavi-dashboard-service:latest` |
| Entity Registry Service | `entity-registry-service/` | Home inventory primitives and discovery surface | `homenavi-entity-registry-service:latest` |
| Device Hub | `device-hub/` | Device inventory, HDP bridge over MQTT, metadata/state fan-out | `homenavi-device-hub:latest` |
| History Service | `history-service/` | HDP device state persistence + query API for charts | `homenavi-history-service:latest` |
| Automation Service | `automation-service/` | Automations/workflows service | `homenavi-automation-service:latest` |
| Weather Service | `weather-service/` | Cached weather API (OpenWeather) | `homenavi-weather-service:latest` |
| Email Service | `email-service/` | Sending verification / notification emails | `homenavi-email-service:latest` |
| Profile Picture | `profile-picture-service/` | Avatar upload & processing | `homenavi-profile-picture-service:latest` |
| Echo Service | `echo-service/` | WebSocket echo & diagnostic tool | `homenavi-echo-service:latest` |
| Zigbee Adapter | `zigbee-adapter/` | Zigbee2MQTT adapter emitting/consuming HDP | `homenavi-zigbee-adapter:latest` |
| Thread Adapter | `thread-adapter/` | Thread adapter placeholder (HDP hello/status/pairing) | `homenavi-thread-adapter:latest` |
| Frontend | `frontend/` | SPA & PWA client | `homenavi-frontend:latest` |

Support:
* `nginx/` reverse proxy templates.
* `prometheus/` scrape config.
* `mosquitto/` local MQTT broker config + data dirs for Device Hub adapters.
* `keys/` (DO NOT COMMIT PRIVATE KEYS IN PRODUCTION REPOS).
* `doc/` guides and design docs.

---

## 6. 🔒 Security & Auth
Implemented:
* RS256 JWT (private signing in Auth Service, public verification at gateway).
* Email-based 2FA workflow (TOTP planned).
* Account & 2FA attempt lockouts (structured 423 responses with remaining time).
* Rate limiting (per-route & global) with Redis.
* Standard JSON error format: `{ "error": string, "code"?: int, ... }` plus optional `reason`, `lockout_remaining`, `unlock_at`.

---

## 7. 📊 Observability
* Metrics: Prometheus scrape (gateway, Go runtime, device hub, etc.).
* Tracing: Jaeger via OpenTelemetry exporters.
* Correlation: Request IDs / correlation IDs propagated across hops.
* Health: Expose `/health` (liveness/readiness separation recommended for prod).
* Device Hub exports its own `/metrics` endpoint and participates in the same trace pipeline.

---

## 8. ⚡ WebSockets & Real‑Time
* Gateway upgrades authenticated using existing JWT (cookie-based flow supported).
* Echo service demonstrates publishing & latency characteristics.
* Device Hub uses MQTT topics for adapter input/output and connects via WebSockets to the UI.
* Foundation for future real-time device state, automation events, and notifications.

Test: `python3 test-websocket.py` (see root script).

---

## 9. 🔌 Extending the Platform
Integrations are **containers** that expose a manifest and optional UI surfaces:

- The integration runs in the same Docker network as the stack.
- `integration-proxy` reads `integrations/config/installed.yaml` and proxies `/integrations/<id>/...`.
- The dashboard catalog merges integration widgets from `GET /integrations/registry.json`.

Template repository (source of truth):

- https://github.com/PetoAdam/homenavi-integration-template

Current runtime model:

- Integrations publish `/.well-known/homenavi-integration.json` (manifest).
- UI surfaces are rendered in sandboxed iframes (tab + widget).
- Same‑origin assets are served under `/integrations/<id>/...` via the proxy.
- Automation actions execute by calling the integration container directly on the internal Docker network (not via `/integrations/<id>/...`).

### Third-party integration development

Third-party integrations should be built in their own repos using the official template:

- https://github.com/PetoAdam/homenavi-integration-template

Design references for next-gen integrations (devices + automations + UI):

- `doc/integration_device_and_automation_extensions.md`
- `doc/poc_lg_thinq_integration_v2.md`

Recommended workflow:

1) Implement your integration and keep metadata in `marketplace/metadata.json`.
2) Add the centralized CI actions from this repo:
	- Verify: `PetoAdam/homenavi/.github/actions/integration-verify@main`
	- Release: `PetoAdam/homenavi/.github/actions/integration-release@main`
3) Tag a release (`vX.Y.Z`). The release workflow builds the image and publishes to the marketplace.

Release hardening (CI):

- `verify.yml` is the primary quality gate (manifest/structure checks, tests, `go vet`, `gosec`, Docker build, and Trivy scan).
- `release.yml` runs `verify.yml` as a required stage before publishing.
- The shared `PetoAdam/homenavi/.github/actions/integration-release@main` action also enforces a central verify gate (`integration-verify` + `go vet` + `gosec`) so release validation cannot be bypassed by repo-local workflow edits.
- Before marketplace publish, release enforces uniqueness checks, emits SBOM + provenance, and signs image digests with keyless Cosign.

Marketplace publishing uses GitHub OIDC tokens (no repo secrets). The marketplace validates:

- The OIDC token is from GitHub Actions for the tagging workflow.
- The tag matches the payload `version` and `release_tag`.
- The repo has a successful `verify.yml` workflow run for the tagged commit.

Make sure your integration repo includes a `verify.yml` workflow and grants `id-token: write` in the release workflow so the OIDC token can be requested.

Security note: when compose-managed installs are enabled, `integration-proxy` runs with Docker socket access and elevated privileges. Treat it as a high‑trust service and restrict access accordingly.

JWT bootstrap behavior:

- Docker Compose runs a one-shot `jwt-bootstrap` service before JWT-consuming services start. It verifies `keys/jwt_private.pem` and `keys/jwt_public.pem`, and generates them if needed via [scripts/ensure-jwt-keys.sh](scripts/ensure-jwt-keys.sh).
- Helm/Kubernetes can do the same in-cluster: if no JWT secret is supplied, the pre-install/pre-upgrade hook in [helm/homenavi/templates/jwt-bootstrap-job.yaml](helm/homenavi/templates/jwt-bootstrap-job.yaml) creates the secret on first install and reuses it on later upgrades.

### Integration proxy installation (recommended)

Use the Admin → Integrations UI to install integrations from the marketplace and manage secrets. The proxy updates [integrations/config/installed.yaml](integrations/config/installed.yaml) automatically.

Runtime policy: use one integration lifecycle runtime per environment.

- Compose-based environment: manage integrations via Compose.
- Kubernetes/Helm-based environment: manage integrations via Helm/Kubernetes artifacts.
- Mixed runtime installs in a single environment are not supported.

First-party integrations (for example Spotify and LG ThinQ) publish both `deployment_artifacts.compose` and `deployment_artifacts.helm` so install/update behavior is parity-first across Compose and Helm runtime modes.

The Helm chart defaults marketplace-backed integration installs to the public Homenavi marketplace at `marketplace.homenavi.org`. End users do not need to run the marketplace in their own cluster for normal installs. If you want Helm installs to resolve metadata from a local marketplace deployment for development, override `INTEGRATIONS_MARKETPLACE_API_BASE` as shown in [doc/minikube_helm_mvp_runbook.md](doc/minikube_helm_mvp_runbook.md).

Installed integrations track their installed `version` (and `auto_update` policy) in `installed.yaml`. Homenavi compares the installed version to the marketplace version (semver) to surface **Update available** and to support **Auto-update**.

If you run custom integrations manually, ensure the container is on the same Docker network and then use Admin → Integrations to add or refresh the entry.

### Integration updates (admin-managed)

- The Admin → Integrations UI shows installed vs latest marketplace version and provides an Update button.
- Updates run asynchronously (queued) and the UI shows progress via the same status surface used during installs.
- Auto-update can be enabled per integration; `integration-proxy` periodically checks the marketplace and applies updates when available.

### Helm installation

An initial Helm chart scaffold is available at [helm/homenavi](helm/homenavi).

Released Helm charts are published to GHCR as OCI artifacts. For tagged releases, install directly from:

- `oci://ghcr.io/petoadam/charts/homenavi`

Example:

```bash
helm install homenavi oci://ghcr.io/petoadam/charts/homenavi \
	--version X.Y.Z \
	-n homenavi --create-namespace
```

The release chart defaults service image tags to the chart `appVersion`, so a chart release and its container images stay aligned by default.

For the current MVP goal (local Minikube Helm for core + marketplace), use the runbook at [doc/minikube_helm_mvp_runbook.md](doc/minikube_helm_mvp_runbook.md).

The local marketplace deployment in that runbook is for development and end-to-end testing only. In normal homelab installs, Homenavi uses the central marketplace service.

For single-namespace MVP deployment in one step, run [scripts/deploy-minikube.sh](scripts/deploy-minikube.sh).

The script supports Kubernetes-native secret ingestion from env files (for Helm deployments), and optional Mosquitto bridge enablement:

```bash
./scripts/deploy-minikube.sh --env-file /home/adam/Projects/homenavi/.env
./scripts/deploy-minikube.sh --with-bridge --bridge-config-file /home/adam/Projects/homenavi/mosquitto/config/conf.d/bridge.conf
./scripts/deploy-minikube.sh --start-port-forwards
```

For a safe starter template, use [k8s/secrets/homenavi.env.example](k8s/secrets/homenavi.env.example).

Default port-forward targets from the deploy script are stable for easier testing:

- frontend: `http://localhost:50001`
- marketplace: `http://localhost:50010`

Legacy alias [scripts/deploy-minikube-planes.sh](scripts/deploy-minikube-planes.sh) now redirects to the current single-namespace deploy script.

Quick start:

```bash
helm upgrade --install homenavi ./helm/homenavi -n homenavi --create-namespace
```

Validation:

```bash
helm lint ./helm/homenavi
helm template homenavi ./helm/homenavi > /tmp/homenavi-rendered.yaml
```

### GitOps note for Kubernetes (ArgoCD/Flux)

For Kubernetes GitOps deployments, manage Homenavi and integrations in your GitOps repository (for example ArgoCD/Flux manifests/apps) and let reconciliation apply changes.

In this mode, do not use the marketplace install/update actions in the Homenavi UI as your source of truth.

This approach works when your GitOps repository includes the required integration artifacts/references and desired release configuration for each integration.

### Integration secrets (admin-managed)

Integrations can declare the secrets they require in the manifest via a `secrets` array. The Admin → Integrations page uses this list to render write-only secret fields and sends values to each integration’s admin endpoint.

Integrations may also expose a **setup UI** flow (in addition to or instead of secrets) via `/api/admin/setup`.

Each integration stores secrets in its own file (default `config/integration.secrets.json` in the integration repo/container, configurable with `INTEGRATION_SECRETS_PATH`). This prevents integrations from seeing each other’s secrets.

See the detailed architecture/roadmap: [doc/dashboard_widgets_integrations_marketplace_roadmap.md](doc/dashboard_widgets_integrations_marketplace_roadmap.md).

---

## 10. 🗺️ Roadmap (Condensed)

Mid Term:
* More adapters (Matter/Z-Wave/BLE) and cloud integrations
* Automation: versioning, richer step library, editor UX improvements
* Scheduling upgrades (sunrise/sunset and other home-aware schedules)
* Scenes & home modes (Away / Night / Eco)
* Third-party integrations groundwork (catalog, sandboxing, verification model)
* AI assistant service (local or cloud) for docs/config/dev support

Long Term:
* Edge node agent & secure tunneling
* Energy analytics & occupancy inference
* Plugin SDK + extension marketplace

---

## 11. ⚙️ Configuration & Environment
Environment variables (selected):
* `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH`
* Integrations / marketplace:
	* `INTEGRATIONS_MARKETPLACE_API_BASE` (defaults to `https://marketplace.homenavi.org`)
	* `INTEGRATIONS_UPDATE_CHECK_INTERVAL` (defaults to `15m`; set `0` to disable periodic checks)
	* `INTEGRATIONS_COMPOSE_ENABLED` (enables compose-managed install/update)
	* `INTEGRATIONS_COMPOSE_PULL_TIMEOUT` (defaults to `2m`, used for slow pulls)
	* `INTEGRATIONS_RUNTIME_MODE` (example: `compose`, `helm`, or `gitops` depending on deployment model)
* Database connection vars (PostgreSQL)
* Redis host/port
* Email provider / SMTP credentials (for Email Service)
* Weather:
	* `OPENWEATHER_API_KEY` (required for real weather data)
	* `WEATHER_CACHE_TTL_MINUTES` (optional, defaults to 15)

Example: `cp .env.example .env` then edit. In production avoid storing secrets directly in env files—use a secrets manager.

Key Management:
```sh
mkdir -p keys
openssl genpkey -algorithm RSA -out keys/jwt_private.pem -pkeyopt rsa_keygen_bits:2048
openssl rsa -pubout -in keys/jwt_private.pem -out keys/jwt_public.pem
```

---

## 12. 📦 CI/CD
* GitHub Actions per-service Docker build pipelines.
* Builds produce Docker images and upload them as artifacts (image tarballs) for download/testing.
* Future: Add lint (golangci-lint), security scanning (gosec, trivy), frontend tests.

---

## 13. 🤝 Contributing & Community
Contributions welcome:
1. Fork & branch
2. Make focused changes (tests appreciated)
3. Open PR with rationale & scope

Discussions / Discord: (coming soon)
Issues: https://github.com/PetoAdam/homenavi/issues

---

## 14. ❓ FAQ
**Can I run it on a Raspberry Pi?** Yes—multi-arch images are the target; optimize build flags if needed.

**Is it production ready?** Homenavi is under active development. The core authentication and user management features are stable, and device + automation layers are implemented, but the platform is still evolving—review the code for your specific use case.

**Can I add my own device protocol now?** Yes, via a custom service publishing REST/WS endpoints through the gateway. The platform is designed to support new adapters and integrations with minimal changes.

**Does it support real-time updates?** Yes—WebSockets already integrated; domain events layer planned.

**Can I build my own automation engine or dashboard?** Yes—extend the platform with custom services, frontend modules, or plugins. The architecture is intentionally open for extension.

**How do I contribute or request a feature?** Open an issue or PR on GitHub, or join the upcoming Discord community.

**How do I run integration tests?** See `test/` for Python scripts covering device, auth, and WebSocket flows. Most tests require a running stack (`docker compose up`).

---

## 15. License
MIT License © 2025 Adam Peto — See [LICENSE](LICENSE).

### Icon attribution

Font Awesome Free icons are used in the UI. Font Awesome is licensed under CC BY 4.0: https://fontawesome.com/license/free

---

> This README describes current capabilities and the forward-looking smart home direction. Features marked “planned” are not yet implemented but inform architectural choices.

