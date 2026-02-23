# POC: LG ThinQ Integration with Device + Automation Extensions

## Scope

POC targets:
- LG TV
- LG Washer

Expected outcomes:
- both devices appear as ERS devices (auto-import via HDP)
- devices are controllable through HDP commands
- map placement works (ERS metadata)
- integration provides custom UI + widgets
- integration contributes automation actions

---

## POC components

## 1) `homenavi-lg-thinq` integration service

Contains:
- OAuth/session + token refresh with LG APIs
- device sync worker
- HDP bridge worker
- automation execute endpoint
- optional custom tab and widgets

## 2) Device sync worker

Every N seconds:
1. Pull LG homes/devices.
2. For each supported device, map to HDP ID:
   - `lgthinq/<homeId>/<deviceId>`
3. Publish retained HDP metadata + state.

## 3) HDP command worker

Subscribes to:
- `homenavi/hdp/device/command/lgthinq/#`

Maps command payloads to LG API calls, publishes:
- `command_result`
- updated `state`

---

## Device mapping (POC)

## LG TV

HDP metadata sample:

```json
{
  "schema": "hdp.v1",
  "type": "metadata",
  "device_id": "lgthinq/homeA/tv123",
  "protocol": "lgthinq",
  "manufacturer": "LG",
  "model": "OLED C2",
  "icon": "tv",
  "online": true,
  "capabilities": [
    "switch",
    "media.playback",
    "media.volume",
    "media.input"
  ],
  "ts": 1760000000000
}
```

HDP state sample:

```json
{
  "schema": "hdp.v1",
  "type": "state",
  "device_id": "lgthinq/homeA/tv123",
  "state": {
    "power": "on",
    "volume": 14,
    "muted": false,
    "input": "hdmi1",
    "playback": "paused"
  },
  "ts": 1760000000001
}
```

Commands supported:
- `set_state` with args:
  - `power: on|off`
  - `volume: 0..100`
  - `muted: bool`
  - `input: hdmi1|hdmi2|...`
  - `playback: play|pause|stop`

## LG Washer

HDP metadata sample:

```json
{
  "schema": "hdp.v1",
  "type": "metadata",
  "device_id": "lgthinq/homeA/washer77",
  "protocol": "lgthinq",
  "manufacturer": "LG",
  "model": "Washer X",
  "icon": "washing-machine",
  "online": true,
  "capabilities": [
    "appliance.washer.status",
    "appliance.washer.control"
  ],
  "ts": 1760000000100
}
```

HDP state sample:

```json
{
  "schema": "hdp.v1",
  "type": "state",
  "device_id": "lgthinq/homeA/washer77",
  "state": {
    "run_state": "running",
    "cycle": "cotton",
    "remaining_min": 43,
    "door_locked": true,
    "error_code": ""
  },
  "ts": 1760000000101
}
```

Commands supported:
- `set_state` with args:
  - `start: true`
  - `pause: true`
  - `stop: true`
  - `cycle: <preset>`

(Exact command support depends on LG API/device permissions.)

---

## ERS and map behavior

No direct ERS writes by integration.

Flow:
1. Integration publishes HDP metadata/state.
2. ERS auto-import creates canonical entities bound by `hdp_external_ids`.
3. User assigns room/tags and map location in existing ERS-powered UI.
4. Device appears in map/widgets/automation selectors using ERS identity + HDP realtime merge.

This preserves your existing architectural boundary.

---

## Automation step catalog (POC)

Integration exposes `/.well-known/homenavi-automation-steps.json`.

Example actions:
1. `lgthinq.tv.set_power`
2. `lgthinq.tv.set_input`
3. `lgthinq.tv.set_volume`
4. `lgthinq.washer.start_cycle`
5. `lgthinq.washer.pause`

Example condition:
- `lgthinq.washer.run_state_is`

Example trigger:
- `lgthinq.washer.cycle_finished`

Automation-service executes these by calling the integration container directly on the internal Docker network.

---

## Pairing model for LG ThinQ

Recommended: **native app pairing + sync**.

- User connects appliances in LG ThinQ app.
- In Homenavi Admin → Integrations, user provides LG credentials/token.
- Integration sync worker imports devices.

Optional in-Homenavi pairing UI can be added later, but is not required for POC.

---

## POC manifest extensions

```json
{
  "device_extension": {
    "enabled": true,
    "provider_id": "lg-thinq",
    "protocol": "lgthinq",
    "discovery_mode": "sync",
    "supports_pairing": false
  },
  "automation_extension": {
    "enabled": true,
    "steps_catalog_url": "/.well-known/homenavi-automation-steps.json",
    "execute_endpoint": "/api/automation/execute"
  }
}
```

---

## POC acceptance criteria

1. TV + washer appear in ERS devices list after sync.
2. Both devices emit realtime HDP state updates.
3. TV command (power/volume/input) works end-to-end from Homenavi UI.
4. Washer command (start/pause) works end-to-end.
5. Devices can be placed on map and survive restart.
6. At least 2 automation actions from LG catalog run successfully.

---

## Risks and mitigations

- Vendor API instability/rate limits:
  - add caching/backoff, idempotent command handling
- Capability mismatch across models:
  - capability negotiation per device; hide unsupported actions
- Token expiry:
  - robust refresh + secret rotation support
- Long-running appliance state lag:
  - periodic resync plus event-driven updates when available

This POC keeps your current architecture intact and extends it in a way that scales to other ecosystems (Samsung SmartThings, Home Connect, etc.).