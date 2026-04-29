# Integration Extensions v2: Devices + Automations + UI

## Why this is needed

Current third-party integrations are strong for:
- custom backend logic
- custom sidebar UI
- custom widgets

But for ecosystems like LG ThinQ, we also need integrations to contribute:
- discoverable/syncable controllable devices
- HDP-compatible capabilities + commands
- optional pairing experiences
- automation step catalog + execution
- ERS-compatible placement for map and room/tag workflows

## Design goals

1. Keep ERS as canonical inventory owner.
2. Keep HDP as realtime protocol owner.
3. Let integrations provide device and automation extensions without bypassing security boundaries.
4. Support both pairing models:
   - native vendor app pairing (most cloud ecosystems)
   - optional in-Homenavi pairing UI/forms
5. Reuse existing integration packaging model (`compose/docker-compose.integration.yml`, manifest, OIDC release).

---

## Proposed extension model

### A) Device Extension Contract (new)

Integrations may declare a `device_extension` block in manifest.

Example shape:

```json
{
  "device_extension": {
    "enabled": true,
    "provider_id": "lg-thinq",
    "protocol": "lgthinq",
    "discovery_mode": "sync",
    "supports_pairing": false,
    "capability_schema_url": "/.well-known/homenavi-capabilities.json"
  }
}
```

Semantics:
- `protocol`: HDP protocol prefix for generated device IDs (`lgthinq/<external>`).
- `discovery_mode`:
  - `sync`: integration periodically syncs devices from cloud/vendor API.
  - `pairing`: HomeNavi triggers pairing commands/events.
- `supports_pairing`: whether HomeNavi should expose pairing UX for this integration.

### B) Automation Extension Contract (new)

Integrations may declare `automation_extension` block.

```json
{
  "automation_extension": {
    "enabled": true,
    "steps_catalog_url": "/.well-known/homenavi-automation-steps.json",
    "execute_endpoint": "/api/automation/execute"
  }
}
```

Semantics:
- `steps_catalog_url`: static/dynamic step definitions (actions/triggers/conditions).
- `execute_endpoint`: integration endpoint called by automation-service on the internal Docker network.

Important policy:
- Automation extension steps are for **integration-specific extras** (account sync, vendor diagnostics, cloud-side routines, etc.).
- Device-oriented automation should continue using core HDP/ERS workflow nodes (`trigger.device_state`, `action.send_command`, selectors).

### C) Runtime capability contribution

Each integration device publishes HDP `metadata` with a normalized capability set:
- `capabilities`: array of capability IDs
- optional `commands` hints
- protocol-native details in `meta` for adapter internals only

This keeps frontend + automation generic while still extensible.

---

## New core responsibilities

## 1) Integration Device Bridge (in integration process)

Each device-capable integration runs a bridge worker that:
- pulls/syncs vendor devices
- maps to HDP device IDs (`protocol/external`)
- publishes HDP retained `metadata` + `state`
- subscribes to HDP command topics for its own protocol/devices
- emits `command_result` and events

For cloud integrations, this is usually enough (no pairing command UX needed).

## 2) Integration Registry metadata expansion (integration-proxy)

Registry output should include extension flags:
- has device extension
- has automation extension
- pairing support

This allows frontend/admin to show capabilities and setup guidance.

## 3) Automation-service integration step executor (integration-only extras)

Automation-service can execute `integration_action` steps only for integration-specific features (non-device controls).

Execution flow:
1. Workflow step references integration ID + action ID.
2. (Optional) automation-service reads integration metadata from the integration-proxy registry.
3. automation-service calls the integration container directly (e.g. `http://<integrationId>:8099<execute_endpoint>`).
4. Integration returns success/error + optional outputs.

## 4) ERS mapping remains canonical

No integration writes directly to ERS DB.

Instead:
- integration emits HDP metadata/state
- existing ERS auto-import binds discovered HDP IDs into ERS entities
- naming/rooms/tags/map placement remain ERS-owned

---

## Pairing and sync modes

### Mode 1: Native app pairing + sync (recommended default)

For ecosystems like LG ThinQ:
- user pairs devices in LG app
- integration authenticates once
- sync job discovers devices and mirrors them into HDP
- ERS auto-import creates/binds entities

### Mode 2: In-Homenavi pairing (optional)

If integration supports local onboarding:
- expose pairing config schema (fields, QR, steps)
- HomeNavi sends HDP pairing command to integration protocol
- integration emits pairing progress events

#### Pairing preset UI component contract

To keep pairing UX consistent and usable across protocols, integrations/adapters should define pairing flows using preset UI components instead of custom frontend logic:

- `PresetStepTimeline`: staged flow cards (`flow.steps[]` with stage/status matching)
- `PresetStatusPill`: current pairing status label
- `PresetCountdown`: optional timeout display from session `expires_at`
- `PresetHintsInline`: inline instructions/hints from schema
- `PresetMetadataPreview`: metadata summary after detection/join
- `PresetActionRow`: start/stop/retry action row with adapter-defined labels

Design constraint:
- presets must align with existing Add Device pairing modal structure, spacing, and interaction patterns; protocol adapters provide data/schema, not custom UI branching.

---

## Security model

1. Integration secrets remain write-only via integration admin endpoint.
2. Integration command execution is authorized via internal service token + role checks.
3. Allowed automation actions are explicit from `steps_catalog` (deny by default).
4. HDP topic namespace is protocol-scoped; integration only handles its own protocol IDs.
5. Keep release gates: verify + go vet + gosec + Trivy + signature.

---

## Data contracts to add (v2)

1. Manifest schema additions:
- `device_extension`
- `automation_extension`

2. Capability schema:
- common capability IDs (switch, media.playback, washer.cycle, temperature, etc.)
- optional per-capability command args schema

3. Automation steps schema:
- `action`, `trigger`, `condition`
- input schema
- output schema
- target selector support (`device`, `room`, `tag`)

---

## Suggested rollout

Phase 1:
- manifest schema extensions
- integration-proxy registry exposure
- docs + template updates

Phase 2:
- automation-service `integration_action` executor
- frontend automation editor support for integration step catalog

Phase 3:
- optional pairing schema and pairing UI for integrations
- richer capability normalization + validation

---

## What this gives you

With this model, a third-party integration can provide all of these at once:
- backend API + cloud sync logic
- custom tab UI + widgets
- controllable HDP devices visible in ERS/device-map
- automation steps usable in first-party workflow engine

while preserving Homenavi architectural boundaries (ERS canonical inventory, HDP realtime protocol).