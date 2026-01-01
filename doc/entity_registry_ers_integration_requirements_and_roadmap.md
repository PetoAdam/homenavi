# Entity Registry (ERS) Integration — Requirements + Detailed Roadmap

This document defines the requirements/featureset for integrating a canonical Home Entity Model into Homenavi, implemented as `entity-registry-service`, and a detailed rollout plan.

## Naming

Service name (docker/compose + discovery): `entity-registry-service`

## Scope boundaries (explicit)

- Single-home only for now.
- `/api/ers/*` is resident-only.
- Entity Registry is REST-first.
- Realtime remains via HDP MQTT-over-WS (`/ws/hdp`) and automation run streams.
- Automations store selectors only; resolution happens at run time.

## Goals

- Provide a canonical, protocol-agnostic model for: rooms, tags, logical devices, and dynamic group membership.
- Enable device grouping and targeting for automations and UI via a selector language (`tag:kitchen`, `room:living`).
- Keep HDP as the source of truth for realtime state/events, pairing, and protocol adapters.

## Core concepts and requirements

### Entities

**LogicalDevice** (Entity Registry-level)
- Required fields:
  - `id` (stable)
  - `name`
- Optional fields:
  - `room_id`
  - `tags[]`
  - `description`
  - `created_at`, `updated_at`

**Binding**
- Purpose: connect a LogicalDevice to one or more physical identifiers.
- Minimum binding needed now:
  - `hdp_external_id` (string; may include slashes, e.g. `zigbee/0x...`)

**Room**
- `id`, `name`, optional ordering metadata.

**Tag**
- Canonical tags power hashtag-style grouping.
- Tag membership may be implemented as:
  - device→tags (device has tags array), or
  - tag→members (explicit membership table)

### Selector resolution

Selector syntax (v1):
- `tag:<slug>`
- `room:<slug-or-id>`

Required endpoint:
- `POST /api/ers/selectors/resolve`
  - Input: selector string
  - Output: resolved targets (at minimum: list of HDP external IDs; optionally also Entity Registry IDs)

Non-goals for v1 (can be phase-2+):
- full boolean expressions (`and/or/not`)
- capability predicates (e.g. `cap:light`)
- history-based selectors

### Authorization

- JWT RS256 via gateway, same model as other resident-only APIs.
- No public endpoints for Entity Registry in v1.

### Data consistency

- HDP remains authoritative for:
  - pairing, integrations
  - realtime state/events
- Entity Registry becomes authoritative for:
  - naming, rooms, tags, and selectors
- Bindings must be tolerant of HDP device churn:
  - device disappears from HDP: binding remains but resolves to empty until it reappears

## External API (target state)

All resident-only:
- `GET /api/ers/home`
- `GET|POST /api/ers/rooms`
- `PATCH|DELETE /api/ers/rooms/{room_id}`
- `GET|POST /api/ers/tags`
- `DELETE /api/ers/tags/{tag_id}`
- `PUT /api/ers/tags/{tag_id}/members`
- `GET|POST /api/ers/devices`
- `GET|PATCH|DELETE /api/ers/devices/{device_id}`
- `PUT /api/ers/devices/{device_id}/bindings/hdp`
- `POST /api/ers/selectors/resolve`

## Service integration points

### API Gateway

Add: `api-gateway/config/routes/ers.yaml`
- `/api/ers/*` → `http://entity-registry-service:<port>/api/ers/*`

### Frontend

- Device list becomes a projection of Entity Registry logical devices, enriched with realtime state from `/ws/hdp`.
- Pairing flow stays against `/api/hdp/*`.
- Tag/room management UI reads/writes Entity Registry.

### Automation service

- Support node targets:
  - `targets: { type: "device", ids: [...] }`
  - `targets: { type: "selector", selector: "tag:kitchen" }`
- At run time:
  - resolve selector by calling `POST /api/ers/selectors/resolve`
  - execute against resolved HDP IDs

## Detailed roadmap

### Milestone 0 — Contract lock (1–2 days)
- Confirm:
  - selector strings: `tag:kitchen`, `room:living`
  - resolve output format (dual output -> resolve HDP too)
  - API path versioning (plain `/api/ers` for now)

Deliverables:
- Final API spec (OpenAPI optional).

### Milestone 1 — `entity-registry-service` scaffold (2–4 days)
- Create Go service skeleton + Dockerfile + health endpoint.
- Add Postgres migrations (legacy structure does not need to work anymore):
  - rooms
  - tags
  - devices
  - device_tags (if using join table)
  - device_bindings
- Make sure to use GORM and no explicit SQL

Deliverables:
- Service runs locally under compose.

### Milestone 2 — Read APIs + minimal write APIs (3–6 days)
- Implement:
  - rooms CRUD
  - tags CRUD (+ membership update)
  - devices CRUD
  - bind HDP ID (device_id) to device

Deliverables:
- Functional REST surface behind gateway.

### Milestone 3 — Selector resolver v1 (2–4 days)
- Implement `POST /api/ers/selectors/resolve`:
  - `tag:<slug>` resolves to bound HDP IDs of devices with that tag
  - `room:<id>` resolves to bound HDP IDs of devices in that room

Deliverables:
- Resolver used by at least one client (automation or simple CLI).

### Milestone 4 — Frontend integration (4–8 days)
- Add Entity Registry client service module.
- Update Devices page:
  - read devices from `/api/ers/devices`
  - keep using `/api/hdp/*` for pairing/integrations
  - keep realtime via `/ws/hdp` and map by binding

Deliverables:
- Main device list driven by Entity Registry.

### Milestone 5 — Automation integration (3–6 days)
- Update automation-service:
  - accept selector-target nodes
  - at run time resolve selectors

Deliverables:
- An automation can target `tag:kitchen`.

### Milestone 6 — Migration + cleanup (time-box)
- Backfill strategy: Manual: user creates logical devices and binds them - old data removed

Deliverables:
- Stable day-to-day UX using Entity Registry.

## Acceptance criteria

- Entity Registry endpoints are resident-only and reachable via nginx → gateway.
- Frontend can:
  - show devices grouped by room/tag (Entity Registry)
  - show realtime state updates (HDP via `/ws/hdp`)
- Automation can target a selector and execute on resolved HDP devices.
- No breaking changes to existing `/api/hdp/*`, `/ws/hdp`, `/api/automation/*`, `/ws/automation/...`.

## Followup:

- Add working Map service on top of ERS
- Custom Dashboard widgets for the home page with per user configuration for them (eg:turn on living room lights widgets)
- AI assistant service + UI integration
- Installer / Admin api in the future
