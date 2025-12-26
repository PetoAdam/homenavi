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
- Exposed via API Gateway under: `/api/hem/*` (path name can stay stable even if the service name changes)

## Proposed external endpoints (after change)

### Entity Registry (HEM) REST (new)

Base: `/api/hem` (access: resident)

Core home model:
- `GET /api/hem/home` (current home summary; single-home can be implicit)
- `GET /api/hem/rooms`
- `POST /api/hem/rooms`
- `PATCH /api/hem/rooms/{room_id}`
- `DELETE /api/hem/rooms/{room_id}`

Tags / hashtags:
- `GET /api/hem/tags`
- `POST /api/hem/tags`
- `DELETE /api/hem/tags/{tag_id}`
- `PUT /api/hem/tags/{tag_id}/members` (bulk set members)

Canonical “entities”:
- `GET /api/hem/devices`
- `POST /api/hem/devices` (create logical device)
- `GET /api/hem/devices/{device_id}`
- `PATCH /api/hem/devices/{device_id}`
- `DELETE /api/hem/devices/{device_id}`

Bindings to the physical/HDP world:
- `PUT /api/hem/devices/{device_id}/bindings/hdp` (bind to HDP external ID(s), e.g. `zigbee/0x...`)

Dynamic groups / selectors (for automation + UI):
- `POST /api/hem/selectors/resolve`
  - Input: selector expression (preferred syntax examples: `tag:kitchen`, `room:living`, plus capability predicates later)
  - Output: resolved device IDs (HEM IDs and/or HDP bindings)

(Optionally later) user widgets:
- `GET /api/hem/widgets` (per-user)
- `POST /api/hem/widgets`
- `PATCH /api/hem/widgets/{widget_id}`
- `DELETE /api/hem/widgets/{widget_id}`

### Existing external endpoints kept as-is

No breaking changes required to keep the system running while HEM rolls out:
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

Add a new route file `api-gateway/config/routes/hem.yaml`:
- `GET|POST|PUT|PATCH|DELETE /api/hem/*` → `http://entity-registry-service:<port>/api/hem/*`

## Data model alignment (minimal)

- HEM device has:
  - stable HEM `device_id`
  - human fields (name, room_id, tags)
  - optional bindings: `hdp_external_id` (string, may include slashes)
- HDP remains the source of realtime device state/events.

## Integration roadmap (phased)

### Phase 0 — Decide contracts
- Confirm external base path: `/api/hem` (or `/api/hem/v1`).
- Confirm whether selectors resolve to HEM IDs, HDP IDs, or both.
- Confirm selector syntax: `tag:kitchen`, `room:living`.
- Confirm that automations store selectors only.

### Phase 1 — Scaffold `entity-registry-service`
- Create service skeleton + Postgres migrations.
- Implement read-only endpoints first (`GET /home`, `/rooms`, `/devices`, `/tags`).

### Phase 2 — Wire through gateway
- Add gateway route config for `/api/hem/*`.
- Add minimal auth middleware expectations (JWT RS256 like others).

### Phase 3 — Frontend reads from HEM (no breaking changes)
- UI uses `/api/hem/*` for rooms/tags/groupings and to display “logical devices”.
- Continue using `/api/hdp/*` for pairing/integrations and `/ws/hdp` for realtime.

### Phase 4 — Add selector resolution + automation integration
- Implement `/api/hem/selectors/resolve`.
- Update `automation-service` to resolve selectors at runtime.

### Phase 5 — Migrations + cleanup
- Migrate any ad-hoc grouping logic in the Frontend into HEM.
- Optionally reduce direct `/api/hdp/devices` usage in UI for the “main” device list (keep for admin/diagnostics/pairing).

## Decisions (answered)

- Access: `/api/hem/*` is resident-only.
- Selector syntax: `tag:kitchen` / `room:living` is preferred.
- Automation persistence: store selectors only (resolution happens at run time).
