# External API Surface (Current)

This document describes the externally-consumable interfaces of the Homenavi stack as it exists in this repo.

Machine-readable contracts:

- `doc/openapi.yaml` documents the HTTP request/response API in OpenAPI 3.1.
- `doc/asyncapi.yaml` documents WebSocket and MQTT-over-WebSocket channels in AsyncAPI 3.0.

## Public ingress (what a client can actually reach)

In the default `docker-compose.yml` config, clients primarily talk to **nginx** on port 80.

- `http://<host>/` → Frontend SPA (container `frontend`)
- `http://<host>/api/...` → API Gateway (container `api-gateway`)
- `ws(s)://<host>/ws/...` → API Gateway websocket reverse-proxy (Upgrade)
- `https://<host>/mcp` → OAuth-protected Streamable HTTP MCP endpoint through API Gateway
- `https://<host>/.well-known/oauth-protected-resource` → MCP protected-resource metadata

Notes:
- The SPA is built from `Frontend/` (capital F). (Case matters on Linux/CI.)
- The API Gateway itself is also published on the host at `http://<host>:8080/` via `docker-compose.yml` (useful for debugging), but nginx is the intended public edge.
- The default Compose broker is EMQX, publishing MQTT on `1883` and MQTT-over-WebSocket on the configured host WebSocket port. The Frontend still uses `/ws/hdp` through nginx → gateway → broker.
- Profile pictures default to S3-compatible object storage via bundled SeaweedFS and are served through `/api/profile-pictures/users/{user_id}`.

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
- `POST /api/auth/profile/upload-url`
- `POST /api/auth/profile/upload-complete`
- `POST /api/auth/profile/upload` (multipart)

OAuth and MCP authorization:
- `GET /.well-known/oauth-authorization-server/api/auth` publishes authorization-server metadata.
- `GET|POST /api/auth/oauth/authorize` begins or completes Authorization Code + PKCE consent.
- `POST /api/auth/oauth/register` dynamically registers a public MCP client. Redirect URIs are restricted to loopback or approved VS Code URIs.
- `POST /api/auth/oauth/token` exchanges an authorization code for an MCP token. mcp-service also uses this endpoint with `grant_type=urn:ietf:params:oauth:grant-type:token-exchange`, a bearer MCP token, gateway resource, and one scope to obtain a delegated token.
- `GET /api/auth/oauth/jwks.json` publishes signing keys.
- `POST /api/auth/oauth/consents` and `DELETE /api/auth/oauth/consents/{clientID}` manage authenticated user consent.

### MCP (`mcp-service`)

- `GET /.well-known/oauth-protected-resource` publishes protected-resource metadata for the public `/mcp` URL.
- `POST /mcp` is the Streamable HTTP MCP endpoint. It requires a resident or admin MCP token after initialization; supported tools and scopes are documented in [MCP](mcp.md).

MCP domain calls use the following dedicated gateway routes. They are not browser API endpoints: `access: delegated` requires a short-lived `homenavi-delegated` token with the API-gateway audience, an active resident/admin session, and the route's exact scope. Raw MCP and browser API tokens are rejected.

| Route family | Methods | Scope |
| --- | --- | --- |
| `/api/mcp/hdp/devices`, `/api/mcp/hdp/devices/*`, `/api/mcp/hdp/integrations`, `/api/mcp/hdp/pairings` | `GET` | `home.devices.read` |
| `/api/mcp/ers/rooms/`, `/api/mcp/ers/groups/*` | `GET` | `home.inventory.read` |
| `/api/mcp/history/state` | `GET` | `home.history.read` |
| `/api/mcp/hdp/device-command-targets/*` | `GET` | `home.devices.write` |
| `/api/mcp/hdp/devices/*/commands` | `POST`, `PATCH` | `home.devices.write` |
| `/api/mcp/ers/group-creates` | `POST` | `home.inventory.write` |
| `/api/mcp/automation/workflows*` | `GET` | `home.automation.read` |
| `/api/mcp/automation/workflows/*/run` | `POST` | `home.automation.execute` |

Public profile picture access:
- `GET /api/profile-pictures/users/{user_id}`
- `GET /api/profile-pictures/users/{user_id}/access-url`

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

### MQTT-over-WS (EMQX default)
- `GET /ws/hdp` (access: auth) → `ws://emqx:8083/mqtt`

Notes:
- The gateway treats `type: websocket-mqtt` the same as `type: websocket` reverse proxy.
- The upstream is provider-driven and defaults to EMQX for both Compose and Helm deployments.
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
