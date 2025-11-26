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

Welcome to Homenavi – your open, hackable smart home solution. Built with a modern microservices architecture, Homenavi is designed for tinkerers, makers, and pros who want full control and easy extensibility.

---

## Table of Contents
1. Why Homenavi
2. Architecture Overview
3. Smart Home Vision (Upcoming Modules)
4. Quickstart
5. Service Directory
6. Security & Auth
7. Observability
8. WebSockets & Real‑Time
9. Extending the Platform
10. Roadmap
11. Configuration & Environment
12. CI/CD
13. Contributing & Community
14. FAQ
15. License

---

## 1. 🚀 Why Homenavi
- **Microservice-first:** Each core feature is its own service – scale, swap, or extend as you like.
- **Modern stack:** Go, React, Python, Docker, and more.
- **Dev-friendly:** Easy to run, hack, and contribute.
- **Open & Transparent:** 100% open source, MIT licensed.
- **Cloud or Home:** Run it on your Raspberry Pi, your server, or in the cloud.
- **Observability built-in:** Prometheus metrics, Jaeger tracing, and request/correlation IDs for easy debugging and monitoring.
- **WebSocket support:** Real-time communication with cookie-based JWT authentication.

---

## 2. 🧩 Architecture Overview

Current Core:
* API Gateway (Go + Chi): Routing, JWT verification, rate limit, WebSocket upgrade.
* Device Hub (Go + MQTT adapters): Central device inventory, capability normalization, and adapter bridge (Zigbee/Matter prototypes, MQTT fan-out).
* Auth Service (Go): Login, password management, 2FA (email now, TOTP coming), lockout logic.
* User Service (Go): Profile, roles, administrative user actions.
* Email Service (Go): Outbound verification & notification emails.
* Profile Picture Service (Python): Image handling (avatars, basic processing).
* Echo Service (Python): Real-time WebSocket demo & test surface.
* Frontend (React + Vite + PWA): Auth flows, user management, future device UI.
* Infrastructure: PostgreSQL, Redis, Nginx, Prometheus, Jaeger, (Grafana ready).

Key Design Principles:
* Clear separation of auth vs user domain.
* Stateless services (use Redis / DB for state persistence & coordination).
* Consistent JSON error schema across services.
* Incremental addition of domain (devices, automations) without core rewrites.

---

## 3. 🔮 Smart Home Vision (Planned / Not Yet Implemented)
The platform roadmap includes:
* Device Adapters: Zigbee, Z-Wave, Matter, BLE, MQTT bridge.
* Automation Engine: Rule graph (triggers → conditions → actions) with versioned deployments.
* Scene & Mode Management: Grouped device state snapshots and home modes (Away / Night / Eco).
* Scheduling & Timers: Cron-like and sunrise/sunset aware triggers.
* Presence & Energy Modules: Occupancy inference; energy usage aggregation.
* Plugin SDK: Custom services registering capabilities & metrics automatically with the gateway.
* Edge Nodes: Lightweight agent pushing device state/events to the core cluster.

These are intentionally referenced now to frame the architecture decisions already in place.

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

## 5. 📂 Service Directory (Brief)
| Service | Path | Purpose |
|---------|------|---------|
| API Gateway | `api-gateway/` | Request routing, auth verification, rate limiting, WS proxy |
| Device Hub | `device-hub/` | Device inventory, adapter bridge (MQTT, Zigbee/Matter stubs), metadata/state fan-out |
| Auth Service | `auth-service/` | Credentials, tokens, 2FA, lockout logic |
| User Service | `user-service/` | User profiles, roles, admin operations |
| Email Service | `email-service/` | Sending verification / notification emails |
| Profile Picture | `profile-picture-service/` | Avatar upload & processing |
| Echo Service | `echo-service/` | WebSocket echo & diagnostic tool |
| Frontend | `frontend/` | SPA & PWA client |

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
* Metrics: Prometheus scrape (gateway, Go runtime). Add service-specific domain metrics as features grow.
* Tracing: Jaeger via OpenTelemetry exporters.
* Correlation: Request IDs / correlation IDs propagated across hops.
* Health: Expose `/healthz` (liveness/readiness separation recommended for prod).
* Device Hub exports its own `/metrics` endpoint on :8090 and participates in the same trace pipeline, so add it to Prometheus once you expose the port or run a sidecar scrape job.

---

## 8. ⚡ WebSockets & Real‑Time
* Gateway upgrades authenticated using existing JWT (cookie-based flow supported).
* Echo service demonstrates publishing & latency characteristics.
* Device Hub uses MQTT topics for adapter input/output today and will drive future WebSocket fan-out once device UI is fully wired.
* Foundation for future real-time device state, automation events, and notifications.

Test: `python3 test-websocket.py` (see root script).

---

## 9. 🔌 Extending the Platform
1. Create a new service directory or external repo.
2. Add container to `docker-compose.yml` (or helm chart later).
3. Register route(s) in `api-gateway` config `gateway.yaml` / routes.*.yaml.
4. Expose metrics, health, and (optionally) tracing.
5. Use JWT for auth; validate only needed claims.

Planned: An Extension/Plugin manifest so services self-register capabilities.

---

## 10. 🗺️ Roadmap (Condensed)

Mid Term:
* Device adapter abstraction (MQTT + Zigbee bridge)
* Rule/automation engine MVP
* Scene & scheduling module
* UI dashboards for metrics & device state

Long Term:
* Edge node agent & secure tunneling
* Energy analytics & occupancy inference
* Plugin SDK & marketplace style discovery

---

## 11. ⚙️ Configuration & Environment
Environment variables (selected):
* `JWT_PRIVATE_KEY_PATH` / `JWT_PUBLIC_KEY_PATH`
* Database connection vars (PostgreSQL)
* Redis host/port
* Email provider / SMTP credentials (for Email Service)

Example: `cp .env.example .env` then edit. In production avoid storing secrets directly in env files—use a secrets manager.

Key Management:
```sh
mkdir -p keys
openssl genpkey -algorithm RSA -out keys/jwt_private.pem -pkeyopt rsa_keygen_bits:2048
openssl rsa -pubout -in keys/jwt_private.pem -out keys/jwt_public.pem
```

---

## 12. 📦 CI/CD
* GitHub Actions per service build pipelines.
* Docker image builds + artifact retention.
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

**Is it production ready?** Homenavi is under active development. The core authentication and user management features are stable, but always review the code for your specific use case. Device & automation layers are forthcoming—treat as early platform.

**Can I add my own device protocol now?** Yes, via a custom service publishing REST/WS endpoints through the gateway.

**Does it support real-time updates?** Yes—WebSockets already integrated; domain events layer planned.

**Will Matter / Zigbee / Z-Wave be supported?** Planned through modular adapters.

---

## 15. License
MIT License © 2025 Adam Peto — See [LICENSE](LICENSE).

---

> This README describes current capabilities and the forward-looking smart home direction. Features marked “planned” are not yet implemented but inform architectural choices.

