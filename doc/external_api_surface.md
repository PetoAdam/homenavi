# External API Surface (Current)

This document describes the externally-consumable interfaces of the Homenavi stack as it exists in this repo.

## Public ingress (what a client can actually reach)

In the default `docker-compose.yml` config, clients primarily talk to **nginx** on port 80.

- `http://<host>/` → Frontend SPA (container `frontend`)
- `http://<host>/api/...` → API Gateway (container `api-gateway`)
- `ws(s)://<host>/ws/...` → API Gateway websocket reverse-proxy (Upgrade)
- `http://<host>/uploads/...` → static file hosting for profile pictures (nginx `alias /uploads/` backed by `profile-pictures` volume)

Notes:
- The SPA is built from `Frontend/` (capital F). (Case matters on Linux/CI.)
- The API Gateway itself is also published on the host at `http://<host>:8080/` via `docker-compose.yml` (useful for debugging), but nginx is the intended public edge.
- Mosquitto ports are published on the host (`1883`, `9001`) in `docker-compose.yml`. These are “external” at the network level, but the Frontend uses `/ws/hdp` through nginx → gateway → mosquitto.

## API Gateway meta endpoints

These are handled directly by `api-gateway` (not via route YAML upstream proxying):

- `GET /health` → `200 ok`
- `GET /metrics` → Prometheus metrics
- `GET /api/gateway/routes` → dumps loaded route config (debug)

## REST endpoints (via API Gateway)

Routes are loaded from `api-gateway/config/routes/*.yaml`.

### Auth service (`auth-service`)

Base: `/api/auth`

Public:
- `POST /api/auth/signup`
- `POST /api/auth/login/start`
- `POST /api/auth/login/finish`
- `POST /api/auth/refresh`
- `POST /api/auth/password/reset/request`
- `POST /api/auth/password/reset/confirm`
- `POST /api/auth/email/verify/request`
- `POST /api/auth/email/verify/confirm`
- `POST /api/auth/2fa/email/request`
- `POST /api/auth/2fa/email/verify`
- `GET /api/auth/oauth/google/login` (Frontend redirects browser here)
- `GET /api/auth/oauth/google/callback`
- `POST /api/auth/oauth/google`

Authenticated:
- `GET /api/auth/me`
- `POST /api/auth/logout`
- `POST /api/auth/delete`
- `POST /api/auth/password/change`
- `POST /api/auth/2fa/setup`
- `POST /api/auth/2fa/verify`
- `POST /api/auth/profile/generate-avatar`
- `POST /api/auth/profile/upload` (multipart)

User management (consolidated in `auth-service`):
- `GET /api/auth/users` (access: resident)
- `GET /api/auth/users/{id}` (access: auth)
- `PATCH /api/auth/users/{id}` (access: auth)
- `POST /api/auth/users/{id}/lockout` (access: admin)

### User service (`user-service`)

Public:
- `POST /api/users` (signup/backing record)
- `POST /api/users/validate` (credential validation helper)

Admin-only ("backup" direct access):
- `GET /api/users`
- `GET /api/users/{id}`
- `PATCH /api/users/{id}`
- `DELETE /api/users/{id}`
- `POST /api/users/{id}/lockout`

### Device Hub / HDP (`device-hub`)

Base: `/api/hdp` (access: resident)

- `GET /api/hdp/devices`
- `POST /api/hdp/devices`
- `GET /api/hdp/devices/*`
- `POST /api/hdp/devices/*`
- `PATCH /api/hdp/devices/*`
- `DELETE /api/hdp/devices/*`
  - Note: `*` is used because HDP IDs can contain slashes (e.g. `zigbee/0x...`).
- `GET /api/hdp/integrations`
- `GET /api/hdp/pairings`
- `POST /api/hdp/pairings`
- `DELETE /api/hdp/pairings`
- `GET /api/hdp/pairing-config`

### History (`history-service`)

Base: `/api/history` (access: resident)

- `GET /api/history/health`
- `GET /api/history/state`
- `GET|POST|PATCH|DELETE /api/history/*` (catch-all)

### Automation (`automation-service`)

Base: `/api/automation` (access: resident)

- `GET /api/automation/health`
- `GET|POST|PUT|PATCH|DELETE /api/automation/*` (catch-all)

## WebSocket endpoints (via API Gateway)

### Generic WS (`echo-service`)
- `GET /ws/echo` (access: auth)

### MQTT-over-WS (Mosquitto)
- `GET /ws/devices` (access: auth) → `ws://mosquitto:9001/`
- `GET /ws/hdp` (access: auth) → `ws://mosquitto:9001/`

Notes:
- The gateway treats `type: websocket-mqtt` the same as `type: websocket` reverse proxy.
- The Frontend uses Paho MQTT over websockets at `/ws/hdp`.

### Automation run stream (`automation-service`)
- `GET /ws/automation/runs/{run_id}` (access: resident) → `ws://automation-service:8094/api/automation/runs/{run_id}/ws`

## Frontend usage map (high level)

- OAuth login: `Frontend/src/components/Auth/AuthModal/AuthModal.jsx` → `/api/auth/oauth/google/login`
- REST clients live in:
  - `Frontend/src/services/authService.js` → `/api/auth/*`
  - `Frontend/src/services/automationService.js` → `/api/automation/*`
  - `Frontend/src/services/historyService.js` → `/api/history/*`
  - `Frontend/src/services/deviceHubService.js` + `Frontend/src/hooks/useDeviceHubDevices.js` → `/api/hdp/*`
- WebSockets:
  - `Frontend/src/components/Automation/hooks/useRunStream.js` → `/ws/automation/runs/{run_id}`
  - `Frontend/src/hooks/useDeviceHubDevices.js` (Paho MQTT) → `/ws/hdp`

## Not currently exposed by gateway route config

The following gateway route files are empty (no externally reachable endpoints added by them):
- `api-gateway/config/routes/admin.yaml`
- `api-gateway/config/routes/routes.yaml`
