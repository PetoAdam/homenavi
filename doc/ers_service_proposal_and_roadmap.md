# Entity Registry Service Proposal + Integration Roadmap

This proposes a new **Entity Registry service** (the canonical Home Entity Model) alongside the existing HDP + automation stack, and how the **external API surface** changes.

Assumptions (based on prior decisions):
- Single-home (for now).
- Entity Registry is protocol-agnostic (not forced to depend on HDP), but may *reference* HDP device IDs for bindings.
- Entity Registry is REST-first initially.
- Realtime remains via MQTT-over-WS (`/ws/hdp`) and automation run streams.

## Proposed service

- Suggested container/service name: `entity-registry-service` (avoids `home-service`)
- Storage: Postgres (new schema/tables; can share the same Postgres instance initially)
- Exposed via API Gateway under: `/api/ers/*`

## Proposed external endpoints (after change)

### Entity Registry (ERS) REST (new)

Base: `/api/ers` (access: resident)

Core home model:
- `GET /api/ers/home` (current home summary; single-home can be implicit)
- `GET /api/ers/rooms`
- `POST /api/ers/rooms`
- `PATCH /api/ers/rooms/{room_id}`
- `DELETE /api/ers/rooms/{room_id}`

Tags / hashtags:
- `GET /api/ers/tags`
- `POST /api/ers/tags`
- `DELETE /api/ers/tags/{tag_id}`
- `PUT /api/ers/tags/{tag_id}/members` (bulk set members)

Canonical “entities”:
- `GET /api/ers/devices`
- `POST /api/ers/devices` (create logical device)
- `GET /api/ers/devices/{device_id}`
- `PATCH /api/ers/devices/{device_id}`
- `DELETE /api/ers/devices/{device_id}`

Bindings to the physical/HDP world:
- `PUT /api/ers/devices/{device_id}/bindings/hdp` (bind to HDP external ID(s), e.g. `zigbee/0x...`)

Dynamic groups / selectors (for automation + UI):
- `POST /api/ers/selectors/resolve`
  - Input: selector expression (preferred syntax examples: `tag:kitchen`, `room:living`, plus capability predicates later)
  - Output: resolved device IDs (ERS IDs and/or HDP bindings)

(Optionally later) user widgets:
- `GET /api/ers/widgets` (per-user)
- `POST /api/ers/widgets`
- `PATCH /api/ers/widgets/{widget_id}`
- `DELETE /api/ers/widgets/{widget_id}`

### Existing external endpoints kept as-is

No breaking changes required to keep the system running while ERS rolls out:
- `/api/hdp/*` (device inventory/pairing/integrations)
- `/ws/hdp` (MQTT-over-WS realtime)
- `/api/automation/*` and `/ws/automation/runs/{run_id}`
- `/api/auth/*` and `/api/auth/users*`
- `/uploads/*`

### Automation evolution (non-breaking first)

Initial: keep automation payloads as they are, but *add* optional selector-based targeting.

- Add support in automation nodes for:
  - `targets: { type: "hdp", ids: [...] }` (current)
  - `targets: { type: "selector", selector: "tag:kitchen" }`
- At run-time, `automation-service` resolves selectors by calling `entity-registry-service`.
- Automations store selectors only (not pre-resolved ID lists).

## API Gateway routing changes

Add a new route file `api-gateway/config/routes/ers.yaml`:
- `GET|POST|PUT|PATCH|DELETE /api/ers/*` → `http://entity-registry-service:<port>/api/ers/*`

## Data model alignment (minimal)

- ERS device has:
  - stable ERS `device_id`
  - human fields (name, room_id, tags)
  - optional bindings: `hdp_external_id` (string, may include slashes)
- HDP remains the source of realtime device state/events.

## Integration roadmap (phased)

### Phase 0 — Decide contracts
- Confirm external base path: `/api/ers` (or `/api/ers/v1`).
- Confirm whether selectors resolve to ERS IDs, HDP IDs, or both.
- Confirm selector syntax: `tag:kitchen`, `room:living`.
- Confirm that automations store selectors only.

### Phase 1 — Scaffold `entity-registry-service`
- Create service skeleton + Postgres migrations.
- Implement read-only endpoints first (`GET /home`, `/rooms`, `/devices`, `/tags`).

### Phase 2 — Wire through gateway
- Add gateway route config for `/api/ers/*`.
- Add minimal auth middleware expectations (JWT RS256 like others).

### Phase 3 — Frontend reads from ERS (no breaking changes)
- UI uses `/api/ers/*` for rooms/tags/groupings and to display “logical devices”.
- Continue using `/api/hdp/*` for pairing/integrations and `/ws/hdp` for realtime.

### Phase 4 — Add selector resolution + automation integration
- Implement `/api/ers/selectors/resolve`.
- Update `automation-service` to resolve selectors at runtime.

### Phase 5 — Migrations + cleanup
- Migrate any ad-hoc grouping logic in the Frontend into ERS.
- Optionally reduce direct `/api/hdp/devices` usage in UI for the “main” device list (keep for admin/diagnostics/pairing).

## Decisions (answered)

- Access: `/api/ers/*` is resident-only.
- Selector syntax: `tag:kitchen` / `room:living` is preferred.
- Automation persistence: store selectors only (resolution happens at run time).
