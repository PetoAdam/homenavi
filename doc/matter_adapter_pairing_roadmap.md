# Homenavi Mock → Zigbee → Matter Adapter Pairing UX Roadmap

## Goal

Validate adapter-defined pairing end-to-end in progressive order:
- mock adapter first (contract and UI validation),
- Zigbee second (real adapter parity),
- Matter last (new commissioning complexity),

then add first-class Matter support to the core Homenavi stack (both:
- Matter over Thread, and
- Matter over direct IP/IPv6 on-network commissioning)

while making pairing UX fully adapter-defined (not hardcoded in frontend), and validating the same contract for Zigbee.

---

## Why this roadmap is needed

Current pairing architecture is close, but not yet fully dynamic:

- Backend already exposes adapter-driven pairing config via `/api/hdp/pairing-config` from adapter hello/status metadata:
  - [homenavi/device-hub/internal/http/adapter_registry.go](device-hub/internal/http/adapter_registry.go)
  - [homenavi/device-hub/internal/http/pairing.go](device-hub/internal/http/pairing.go)
- Frontend pairing flow still contains protocol fallback/hardcoded behavior:
  - [homenavi/frontend/src/components/Devices/AddDeviceModal.jsx](frontend/src/components/Devices/AddDeviceModal.jsx)
  - Includes protocol-specific instruction constants and default protocol fallbacks.
- Integrations/extensions doc indicates where this should evolve next:
  - [homenavi/doc/integration_device_and_automation_extensions.md](doc/integration_device_and_automation_extensions.md)

Outcome target: frontend should render pairing flows from adapter-declared schema, including Matter-specific flows and fields, with no protocol-specific logic embedded in UI components.

Policy decision for this rollout: no backward-compatibility layer for legacy pairing announcements. Adapters must emit the v1 pairing schema (`pairing.schema_version`) to be surfaced as pairable.

---

## Matter commissioning requirements to cover (complete flow)

Based on Matter commissioning references (Google Home Matter primer, Silabs Matter commissioning, and connectedhomeip/chip-tool docs), we must support:

1. **Onboarding input formats**
   - QR payload (`MT:...`)
   - Manual setup code / passcode
   - Optional discriminator input for faster selection

2. **Commissionable discovery methods**
   - BLE discovery + PASE
   - DNS-SD on existing IP network (on-network commissioning)
   - Thread devices discovered via Thread Border Router SRP proxy path

3. **Network paths**
   - **Thread path**: provide/resolve operational dataset + border router connectivity
   - **Direct IPv6/IP path**: on-network pairing (device already on IP medium)

4. **Commissioning lifecycle stages**
   - Discovery
   - PASE
   - Attestation
   - NOC/operational credentials
   - Network provisioning (for Thread/Wi-Fi path)
   - Operational discovery
   - CASE
   - CommissioningComplete

5. **Failure and recovery scenarios**
   - Invalid QR/manual code
   - PASE timeout
   - Attestation failure
   - Thread dataset missing/invalid
   - Border router unavailable
   - Commissioning complete not reached

---

## Architecture direction

## 1) Add dedicated `matter-adapter` service in core stack

Create a new service similar in role to Zigbee adapter:

- subscribes to:
  - `homenavi/hdp/device/command/matter/#`
  - `homenavi/hdp/pairing/command/matter`
- publishes:
  - `homenavi/hdp/device/metadata/matter/<id>`
  - `homenavi/hdp/device/state/matter/<id>`
  - `homenavi/hdp/device/command_result/matter/<id>`
  - `homenavi/hdp/pairing/progress/matter`
  - `homenavi/hdp/adapter/hello`
  - `homenavi/hdp/adapter/status/<adapter-id>`

Design principle: `device-hub` remains orchestrator/registry; protocol-specific commissioning and command translation live in adapter.

## 2) Extend adapter pairing contract from simple profile -> full flow schema

Current `PairingConfig` is mostly:
- label
- supported
- timeout
- instructions
- CTA label

This should evolve into a **versioned pairing schema** announced by adapters in hello/status payload.

Proposed top-level structure in adapter hello/status:

```json
{
  "schema": "hdp.v1",
  "type": "hello",
  "protocol": "matter",
  "features": {
    "supports_pairing": true,
    "supports_interview": true
  },
  "pairing": {
    "schema_version": "1.0",
    "label": "Matter",
    "supported": true,
    "default_timeout_sec": 300,
    "flow": {
      "entry_modes": [
        "qr_code",
        "manual_code",
        "on_network"
      ],
      "forms": [...],
      "steps": [...],
      "capabilities": {
        "camera_scan": true,
        "manual_code": true,
        "thread_dataset_required_for_thread_path": true,
        "on_network_discovery": true
      }
    }
  }
}
```

## 3) Pairing command and progress payloads become typed by flow step

Extend `pairing_command` payload to carry dynamic flow fields:

```json
{
  "schema": "hdp.v1",
  "type": "pairing_command",
  "protocol": "matter",
  "action": "start",
  "timeout": 300,
  "flow_id": "uuid",
  "mode": "qr_code",
  "inputs": {
    "onboarding_payload": "MT:...",
    "manual_code": "",
    "discriminator": 3840,
    "network_path": "thread",
    "thread_operational_dataset": "hex:..."
  }
}
```

Extend `pairing_progress` with machine-readable stage and required-next-input:

```json
{
  "schema": "hdp.v1",
  "type": "pairing_progress",
  "protocol": "matter",
  "flow_id": "uuid",
  "stage": "pase|attestation|network_provisioning|case|completed|failed",
  "status": "in_progress|needs_input|completed|failed|timeout",
  "error_code": "THREAD_DATASET_MISSING",
  "message": "Thread dataset is required for selected path",
  "required_inputs": ["thread_operational_dataset"]
}
```

This lets UI remain generic.

---

## Frontend revamp plan (no hardcoded protocol logic)

## Preset pairing UI components (schema-defined)

To keep UX consistent with the current pairing modal while removing protocol hardcoding, adapters should compose flows from these preset UI components:

1. `PresetStepTimeline`
   - Maps to current pairing stage cards and progress states.
   - Inputs: `flow.steps[]` with `id`, `label`, optional `description`, and `stage/status` matching rules.

2. `PresetStatusPill`
   - Maps to current session status badge.
   - Inputs: normalized session status (`starting`, `active`, `interviewing`, `completed`, `failed`, ...).

3. `PresetCountdown`
   - Maps to current timeout countdown block.
   - Inputs: session `expires_at` + visibility policy (`hide_after_stage` allowed).

4. `PresetHintsInline`
   - Maps to current inline instruction chips.
   - Inputs: `pairing.instructions[]` and optional `flow.hints[]`.

5. `PresetMetadataPreview`
   - Maps to current post-join metadata summary (type/manufacturer/model/icon/device id).
   - Inputs: session metadata and detected device id.

6. `PresetActionRow`
   - Maps to current primary/secondary pairing actions (`start`, `stop`, `retry`).
   - Inputs: CTA labels from adapter schema and action availability by status.

Design/usability rule: preset components must reuse existing Add Device modal layout, spacing, and states so protocol changes do not alter interaction patterns.

## A) Replace protocol hardcoded hints with schema renderer

Refactor [homenavi/frontend/src/components/Devices/AddDeviceModal.jsx](frontend/src/components/Devices/AddDeviceModal.jsx):

- remove hardcoded `PAIRING_INSTRUCTIONS`/protocol defaults from control flow
- render pairing forms, hints, and steps from adapter-provided `pairing.flow`
- require adapters to provide `pairing.schema_version` and `pairing.flow`; do not add protocol fallback logic

## B) Introduce generic Pairing Flow Renderer

New UI module (example path):
- `frontend/src/components/Devices/pairing/PairingFlowRenderer.jsx`

Responsibilities:
- render fields by schema type (`text`, `number`, `password`, `select`, `qr_payload`, `manual_code`)
- support conditional field visibility
- submit typed payload to `startPairing`
- consume realtime `pairing_progress` stages and update step UI

## C) Keep transport details out of UI

UI does not know Thread internals, border router specifics, or Matter command syntax.
It only sends adapter-declared input fields and shows adapter-declared statuses/errors.

---

## Matter adapter functional scope

## Phase M1: pairing-only skeleton

- service scaffold + hello/status
- advertise pairing flow schema
- accept `start/stop` pairing commands
- emit synthetic stage updates (for UI integration)

## Phase M2: real commissioning pipeline

Implement commissioning paths:

1. **QR/manual + Thread path**
   - parse onboarding payload / manual code
   - perform BLE rendezvous and PASE (when required)
   - apply Thread network credentials (dataset)
   - finish commissioning and CASE

2. **On-network direct IPv6/IP path**
   - DNS-SD commissionable discovery on existing network
   - pairing with setup code (+ optional discriminator filters)
   - complete commissioning

3. **Node registration in HDP**
   - publish metadata/state for commissioned nodes
   - map to `matter/<node or stable external id>`

## Phase M3: commands + state sync

- subscribe to `device/command/matter/#`
- translate state operations to Matter clusters/attributes/commands
- publish `command_result` and correlated state updates

---

## Zigbee parity checks (required)

During revamp, validate Zigbee pairing UX also uses adapter schema end-to-end:

1. Zigbee adapter hello/status advertises pairing schema with `schema_version`.
2. Frontend renders Zigbee pairing form/steps from schema, not hardcoded constants.
3. Zigbee pairing command/progress payloads remain compatible with new generic renderer.
4. Existing Zigbee behavior remains unchanged functionally.

Files to verify:
- [homenavi/zigbee-adapter/internal/proto/zigbee/hdp.go](zigbee-adapter/internal/proto/zigbee/hdp.go)
- [homenavi/zigbee-adapter/internal/proto/zigbee/pairing.go](zigbee-adapter/internal/proto/zigbee/pairing.go)
- [homenavi/frontend/src/components/Devices/AddDeviceModal.jsx](frontend/src/components/Devices/AddDeviceModal.jsx)

---

## Environment configuration (`.env`) for initial Matter support

For now, keep Matter runtime config in `.env` as requested.

Suggested variables:

```env
# Core toggle
MATTER_ADAPTER_ENABLED=true

# Adapter identity
MATTER_ADAPTER_ID=matter
MATTER_ADAPTER_LOG_LEVEL=info

# Commissioning defaults
MATTER_DEFAULT_PAIRING_TIMEOUT_SEC=300
MATTER_ENABLE_ON_NETWORK=true
MATTER_ENABLE_BLE=true
MATTER_ENABLE_THREAD=true

# Thread path
MATTER_THREAD_BORDER_ROUTER_HOST=
MATTER_THREAD_BORDER_ROUTER_PORT=49155
MATTER_THREAD_OPERATIONAL_DATASET_HEX=
MATTER_THREAD_DATASET_SOURCE=env

# On-network discovery path
MATTER_DNSSD_DISCOVERY_TIMEOUT_MS=30000
MATTER_COMMISSIONING_INTERFACE=auto

# Security / credentials
MATTER_FABRIC_STORAGE_PATH=/data/matter/fabric
MATTER_TRUST_STORE_PATH=/data/matter/trust
MATTER_FAILSAFE_MAX_SEC=180
```

Note: move sensitive credential material to secrets manager later; `.env` is temporary bootstrap for local/home deployments.

---

## Device-hub and schema evolution tasks

1. Extend pairing schema parsing in:
   - [homenavi/device-hub/internal/http/adapter_registry.go](device-hub/internal/http/adapter_registry.go)
2. Enforce strict schema contract:
   - adapters missing `pairing.schema_version` are not exposed as pairable
3. Add versioned API response from `/api/hdp/pairing-config`:
   - include `schema_version`
   - include optional `flow` object if available
4. Keep `start/stop` endpoints stable while allowing `inputs` payload extension.

5. Validate presets contract:
   - backend response must contain all data required by preset components.
   - UI must not add protocol branches to fill missing adapter schema data.

---

## Testing roadmap (including Kajplats bulbs)

## T1: contract tests (backend/frontend)

- adapter hello/status parsing for `pairing.flow`
- UI renders fields and steps from schema
- no protocol-specific branching required to render Matter flow

## T2: commissioning happy paths

1. **Matter Thread + QR** (Kajplats bulb)
   - start pairing with QR payload
   - adapter asks for/uses Thread dataset
   - commissioning completes
   - device appears in devices list and ERS mapping

2. **Matter Thread + manual code**
   - manual setup code path
   - completion and state updates verified

3. **Matter direct IPv6/on-network**
   - device already discoverable on IP
   - `on_network` flow with setup code (+ optional discriminator)
   - commissioning completes without Thread dataset

## T3: negative paths

- wrong code/discriminator
- Thread dataset missing when thread path selected
- border router unavailable
- timeout and retry behavior

## T4: Zigbee regression

- pairing start/stop/progress unchanged
- UI driven by schema (no hardcoded Zigbee-specific logic)

---

## Implementation phases and deliverables

## Phase 0 — Mock adapter baseline (first step)

Deliverables:
- rename placeholder service to `mock-adapter`
- emit strict pairing schema v1 (`schema_version`, `flow`)
- wire mock pairing progress through preset UI components
- verify Add Device modal works without protocol hardcoding

Exit criteria:
- mock adapter pairing works as the reference adapter for schema and preset UX behavior

## Phase 1 — Zigbee schema parity before Matter

Deliverables:
- Zigbee adapter emits full schema v1 + preset-compatible flow definitions
- frontend renders Zigbee pairing only from adapter schema
- remove remaining Zigbee-specific fallback behavior from pairing modal

Exit criteria:
- Zigbee pairing passes regression on schema-driven UX before Matter work begins

## Phase 2 — Matter adapter MVP pairing

Deliverables:
- `matter-adapter` service scaffold
- hello/status + flow schema aligned to presets
- start/stop + staged progress events
- `.env` config wiring

Exit criteria:
- UI runs through Matter flow via same preset components used by mock and Zigbee

## Phase 3 — Real Matter commissioning paths

Deliverables:
- Thread path commissioning
- direct on-network IPv6 commissioning
- operational node registration into HDP metadata/state

Exit criteria:
- Kajplats bulbs commission successfully through both supported paths where hardware/network allows

## Phase 4 — Command/state + hardening

Deliverables:
- command translation for common clusters (on/off, level, color temp)
- lifecycle correlation + retry tuning
- reliability/perf improvements

Exit criteria:
- stable daily use in home setup; no frequent stuck pairing/command states

---

## Risks and mitigations

1. **Thread BR variability**
   - Mitigation: explicit adapter diagnostics stage and clear required-input errors.

2. **Matter stack complexity in first release**
   - Mitigation: strict MVP scope, then expand cluster support incrementally.

3. **UI schema drift across adapters**
   - Mitigation: schema versioning + JSON schema validation in CI.

4. **Regression in Zigbee pairing**
   - Mitigation: Zigbee parity gate in test matrix before merging.

---

## Definition of done

Matter support is considered ready when:

- Matter adapter is part of core stack and configurable via `.env`.
- Pairing UI is adapter-defined and schema-driven (not protocol-hardcoded).
- Both Thread and direct IPv6/on-network commissioning paths are implemented.
- Zigbee pairing is validated under the same schema-driven UI model.
- Kajplats bulb commissioning test passes at least once for each supported commissioning path in your environment.
