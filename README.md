# Homenavi


**A smart home platform for developers, by developers. Modern, microservice-based, and built to be extended.**

[![Build Frontend Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/frontend_docker_build.yaml)
[![Build User Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/user_service_docker_build.yaml)
[![Build API Gateway Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/api_gateway_docker_build.yaml)
[![Build Auth Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/auth_service_docker_build.yaml)
[![Build Device Hub Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/device_hub_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/device_hub_docker_build.yaml)
[![Build Email Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/email_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/email_service_docker_build.yaml)
[![Build Profile Picture Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/profile_picture_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/profile_picture_service_docker_build.yaml)
[![Build Echo Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/echo_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/echo_service_docker_build.yaml)
[![Build History Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/history_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/history_service_docker_build.yaml)
[![Build Zigbee Adapter Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/zigbee_adapter_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/zigbee_adapter_docker_build.yaml)
[![Build Thread Adapter Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/thread_adapter_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/thread_adapter_docker_build.yaml)
[![Build Automation Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/automation_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/automation_service_docker_build.yaml)
[![Build Entity Registry Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/entity_registry_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/entity_registry_service_docker_build.yaml)
[![Build Weather Service Docker Image](https://github.com/PetoAdam/homenavi/actions/workflows/weather_service_docker_build.yaml/badge.svg)](https://github.com/PetoAdam/homenavi/actions/workflows/weather_service_docker_build.yaml)

Welcome to Homenavi – your open, hackable smart home solution. Built with a modern microservices architecture, Homenavi is designed for tinkerers, makers, and pros who want full control and easy extensibility.

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

Template structure (clean layout for devs):

- `integrations/integration-template-repo/src/backend` → backend server code
- `integrations/integration-template-repo/src/backend/cmd/integration` → backend entrypoint
- `integrations/integration-template-repo/src/frontend` → tab + widget UI code
- `integrations/integration-template-repo/web` → built assets (ui/widgets) + static assets

Current runtime model:

- Integrations publish `/.well-known/homenavi-integration.json` (manifest).
- UI surfaces are rendered in sandboxed iframes (tab + widget).
- Same‑origin assets are served under `/integrations/<id>/...` via the proxy.

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

---

> This README describes current capabilities and the forward-looking smart home direction. Features marked “planned” are not yet implemented but inform architectural choices.

