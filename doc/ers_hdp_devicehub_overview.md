# ERS / HDP / Device Hub — How it works (current)

This document explains how **ERS** (Entity Registry Service / `entity-registry-service`), **HDP** (HomeNavi Device Protocol), and **device-hub** work together in the current codebase.

It also documents how the **frontend** (including Map and Automations UI) and backend services call into these systems.

## Glossary

- **HDP**: HomeNavi Device Protocol. A realtime, protocol-agnostic message contract carried over MQTT topics under `homenavi/hdp/*`.
- **device-hub**: The service that ingests HDP messages from adapters via MQTT and exposes them to clients via HTTP and MQTT-over-WebSocket.
- **ERS** (Entity Registry): Canonical inventory + user metadata: device names, rooms, tags, and map metadata (room geometry + device placement/favorites).
- **HDP `device_id`**: The canonical physical device identifier used by HDP. It is an opaque string shaped like `protocol/<external>`, where `<external>` may itself contain `/`.
- **ERS device**: A canonical inventory record that may bind to one or more HDP external ids via `hdp_external_ids`.

## Responsibilities (the boundary)

### HDP + adapters + device-hub (realtime)

Owns **realtime identity and telemetry**:
- online/offline
- last seen timestamps
- device metadata (manufacturer, model, capabilities, etc.)
- device state
- device events
- device pairing progress
- device commands + command results

Important rule:
- **HDP does not carry user-facing device names**. Names live in ERS.

### ERS / Entity Registry (canonical inventory)

Owns **inventory and user customization**:
- canonical device name
- room list (and device room assignment)
- tags + tag membership
- map layout metadata:
  - room geometry stored in room `meta.map`
  - device placement + “favorite fields” stored in device `meta.map`
- binding between ERS devices and HDP identities (via `hdp_external_ids`)

## Data flow overview

### 1) Adapters → MQTT (HDP)

Adapters (e.g. Zigbee adapter, Thread adapter) publish HDP frames to MQTT topics under `homenavi/hdp/*`.

See [doc/hdp.md](doc/hdp.md) for the exact topic list and payload envelopes.

### 2) device-hub ingests HDP and exposes it to clients

- device-hub subscribes to HDP topics (state, metadata, events, pairing progress).
- device-hub provides an HTTP API under `/api/hdp/*` (via gateway) for:
  - listing devices
  - sending commands
  - pairing start/stop
  - other HDP-facing operations

The frontend connects to **MQTT-over-WebSocket** at `/ws/hdp` (via gateway → mosquitto) to receive realtime HDP traffic.

### 3) ERS auto-import: HDP identity → canonical ERS devices

ERS subscribes to HDP MQTT topics and ensures the canonical inventory matches the physical reality.

The important behaviors:
- When new HDP devices appear, ERS ensures there is a corresponding ERS device and binds it via `hdp_external_ids`.
- When HDP emits `device_removed`, ERS performs a **hard delete** (or unbind if the device had multiple bindings), so ERS does not retain removed devices.

## Public API surface (through gateway)

### ERS (entity-registry-service)

- REST: `/api/ers/*` (resident access)
  - Examples:
    - `GET /api/ers/devices`
    - `GET /api/ers/rooms`
    - `GET /api/ers/tags`
    - `PATCH /api/ers/devices/{device_id}`
    - `PATCH /api/ers/rooms/{room_id}`
    - `POST /api/ers/selectors/resolve`
- WebSocket: `/ws/ers` (resident access)
  - Emits inventory change events (a change-notification stream).
  - The canonical data is still fetched via REST.

### HDP (device-hub + mosquitto)

- REST: `/api/hdp/*` (auth/resident depending on route)
  - Examples: `GET /api/hdp/devices`, `POST /api/hdp/devices/{id}/commands`, pairing endpoints.
- WebSocket MQTT bridge: `/ws/hdp`
  - The frontend uses this to subscribe to HDP topics.

## Frontend: how it uses ERS + HDP

### Realtime devices list (telemetry)

The frontend uses MQTT-over-WebSocket (`/ws/hdp`) to subscribe to HDP topics and build a list of realtime devices.

Key property:
- This realtime device list **does not contain user-facing names**.

### Canonical inventory (names/rooms/tags)

The frontend loads inventory via ERS REST:
- `GET /api/ers/devices`
- `GET /api/ers/rooms`
- `GET /api/ers/tags`

Then it merges the ERS records with realtime HDP telemetry by matching:
- `ersDevice.hdp_external_ids[0]` (as `hdpId`) ↔ realtime device’s `device_id` / `hdpId`

### ERS realtime updates (websocket)

The frontend also connects to ERS websocket `/ws/ers`.

Current behavior:
- On any websocket message, the UI **debounces and refreshes ERS inventory via REST**.
- This keeps the websocket payload small and keeps REST as the canonical data source.

## Map: what it calls

### Reads
- ERS REST for rooms/devices/tags (names + map metadata).
- HDP realtime via `/ws/hdp` for state values and online status.

### Writes
Map editing persists via ERS REST:
- create/rename/delete rooms (`/api/ers/rooms`)
- patch room geometry (`PATCH /api/ers/rooms/{id}` with `meta.map`)
- patch device placement + favorite fields (`PATCH /api/ers/devices/{id}` with `meta.map`)

These writes trigger ERS websocket notifications, which causes other clients to refresh.

## Automations: what it calls

### UI (frontend)
The automations UI uses the merged device list (ERS names + HDP state) to present device options.

### automation-service (backend)
The automation engine must resolve selectors (e.g. `tag:kitchen`, `room:living-room`) into concrete device targets.

Current design:
- automation-service calls ERS REST `POST /api/ers/selectors/resolve`.
- ERS responds with `hdp_external_ids`.
- automation-service then uses those HDP IDs to listen for state / send commands through the HDP layer.

## Do we still call ERS via REST?

Yes, and intentionally.

- `/ws/ers` is a **change notification** stream.
- REST `/api/ers/*` remains the source of truth for fetching and mutating inventory data.

Current ERS REST callers:
- frontend inventory load + all inventory mutations
- automation-service selector resolution (`/api/ers/selectors/resolve`)

Current ERS websocket consumer:
- frontend inventory refresh trigger

## Quick “who owns what?” summary

- **Names, rooms, tags, map layout**: ERS
- **Realtime telemetry, pairing, commands**: HDP + device-hub
- **UI device list**: ERS inventory merged with HDP realtime by `hdpId`
