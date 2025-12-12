# HomeNavi Device Protocol (HDP)

HDP is the internal, protocol-agnostic contract that every adapter, core service, and UI uses. Adapters translate protocol-native payloads (Zigbee2MQTT, Matter, Thread, cloud APIs, cameras, etc.) into HDP messages. Everything above adapters (device-hub, persistence, rule engine, frontend, AI/energy services) consumes only HDP.

## Goals
- One schema for device identity, capabilities, state, commands, events, and pairing across all transports.
- Matter-aligned where possible; extensible for non-Matter domains (cameras, media, HVAC vendors, AI, energy, RTSP/YOLO, custom sensors).
- Stable, versioned envelope; optional vendor extensions.
- Transport-agnostic: MQTT/NATS primary; REST/WS bridges allowed but must emit/ingest HDP payloads.

## Core Concepts
- **Device ID**: `protocol/<adapter>/<external>` or `protocol/<external>`; globally unique; stable. Example: `zigbee/z2m/0x0017880104abcd`. UI-friendly alias lives separately.
- **Endpoint (optional)**: Sub-address within a device (Zigbee endpoint 1-240, Matter endpoint numeric, cloud virtual sensor, camera channel). Present only when a capability is bound to a specific endpoint. Example: endpoint `1` on `zigbee/z2m/0x001788...`.
- **Capabilities**: Namespaced to avoid collisions and to group by domain (e.g., `core.on_off`, `core.level`, `core.color_xy`, `env.temperature`, `env.humidity`, `core.power`, `media.control`, `camera.stream`, `camera.detection`, `robot.vacuum`, `energy.meter`).
- **Attributes (State)**: Key-value pairs under a capability (e.g., `core.on_off.state`, `core.level.brightness`, `env.temperature.value`, `core.power.watts`).
- **Commands**: Write-side actions scoped to a capability (e.g., `core.on_off.set`, `core.level.set_level`, `core.color_xy.set`, `media.control.play`).
- **Events**: Asynchronous notifications (e.g., `sensor.triggered`, `camera.detection.object_detected`, `robot.vacuum.map_updated`).
- **Metadata**: Human/UI hints (name, icon, manufacturer, model, location, widget recommendations) plus capability descriptors (value ranges, units, feature flags) and user overrides.
- **Pairing**: Unified start/stop + progress + config flows per protocol.
- **Observability**: Health/heartbeat, adapter hello/status, metrics events.

## Envelopes (common fields)
Every HDP message uses a small envelope; payload depends on kind.
- `schema`: string, HDP schema ID (e.g., `hdp.v1`)
- `type`: one of `state`, `command`, `command_result`, `event`, `metadata`, `pairing_progress`, `pairing_command`, `hello`, `status`, `metric`
- `device_id`: stable device identifier (absent for hello/pairing-config)
- `endpoint`: optional endpoint/subdevice identifier when the capability is bound to a specific endpoint
- `capability`: capability name (when applicable)
- `ts`: unix ms (producer timestamp)
- `corr`: optional correlation/command id
- `source`: adapter ID (e.g., `zigbee2mqtt`, `thread-adapter`)
- `ext`: optional vendor extension object

## Topics (MQTT/NATS)
Use `homenavi/hdp/<area>/...`. Examples:
- Device state/events: `homenavi/hdp/device/state/{device_id}` (retain last), `homenavi/hdp/device/event/{device_id}`
- Commands (to adapter): `homenavi/hdp/device/command/{device_id}`; command results return on `homenavi/hdp/device/command_result/{device_id}`
- Metadata: `homenavi/hdp/device/metadata/{device_id}` (retain)
- Pairing: commands `homenavi/hdp/pairing/command/{protocol}`, progress `homenavi/hdp/pairing/progress/{protocol}`
- Adapter presence: `homenavi/hdp/adapter/hello`, `homenavi/hdp/adapter/status/{adapter_id}`
- Metrics/health: `homenavi/hdp/metric/{adapter_id}`

### Example payloads (JSON)
- State
  ```json
  {
    "schema": "hdp.v1",
    "type": "state",
    "device_id": "zigbee/z2m/0x0017880104abcd",
    "endpoint": "1",
    "capability": "core.on_off",
    "ts": 1734114123123,
    "state": {"state": "on"}
  }
  ```
- Command
  ```json
  {
    "schema": "hdp.v1",
    "type": "command",
    "device_id": "thread/br/abcd1234",
    "endpoint": "0",
    "capability": "core.level",
    "command": "set_level",
    "args": {"level": 78, "transition_ms": 300},
    "corr": "cmd-123"
  }
  ```
- Command result
  ```json
  {
    "schema": "hdp.v1",
    "type": "command_result",
    "device_id": "thread/br/abcd1234",
    "endpoint": "0",
    "capability": "core.level",
    "corr": "cmd-123",
    "success": true,
    "error": null
  }
  ```
- Event (camera/AI)
  ```json
  {
    "schema": "hdp.v1",
    "type": "event",
    "device_id": "rtsp/cam/front-door",
    "capability": "camera.detection",
    "event": "object_detected",
    "data": {"class": "person", "confidence": 0.93, "bbox": [0.1,0.2,0.5,0.6]}
  }
  ```
- Metadata
  ```json
  {
    "schema": "hdp.v1",
    "type": "metadata",
    "device_id": "zigbee/z2m/0x0017880104abcd",
    "endpoint": "1",
    "name": "Hallway Light",
    "manufacturer": "Philips",
    "model": "LWB010",
    "icon": "lightbulb",
    "user_meta": {
      "room": "Hallway",
      "area": "Downstairs",
      "preferred_name": "Hall Light",
      "icon_override": "lightbulb-outline",
      "widget_overrides": {"core.level": "slider"}
    },
    "capabilities": {
      "core.on_off": {"preferred_widget": "toggle"},
      "core.level": {"range": [1,254], "step": 1, "preferred_widget": "slider"}
    }
  }
  ```
- Pairing command (start)
  ```json
  {
    "schema": "hdp.v1",
    "type": "pairing_command",
    "action": "start",
    "protocol": "zigbee",
    "timeout_sec": 60,
    "metadata": {"name": "Kitchen Light", "icon": "lightbulb"}
  }
  ```
- Pairing progress
  ```json
  {
    "schema": "hdp.v1",
    "type": "pairing_progress",
    "protocol": "thread",
    "stage": "commissioning",
    "status": "in_progress",
    "external_id": "thread/br/abcd1234",
    "ts": 1734114123123
  }
  ```
- Adapter hello/status
  ```json
  {
    "schema": "hdp.v1",
    "type": "hello",
    "adapter_id": "thread-adapter",
    "protocol": "thread",
    "version": "0.2.0",
    "capabilities": ["pairing", "state", "command", "command_result"],
    "health": {"status": "online"}
  }
  ```

## Capability Catalog (starter set, namespaced)
- Core: `core.on_off`, `core.level`, `core.color_xy`, `core.color_temp`, `core.scene`, `core.power`, `core.energy`, `core.contact`, `core.motion`, `core.occupancy`, `core.lock`, `core.thermostat`, `core.fan`, `core.air_quality`.
- Environment: `env.temperature`, `env.humidity`, `env.illuminance`, `env.co2`, `env.pm25`.
- Media: `media.control` (play/pause/seek), `media.volume`, `media.source`.
- Camera/AI: `camera.stream` (rtsp/webrtc url), `camera.detection` (object/person/vehicle), `camera.snapshot`.
- Robotics: `robot.vacuum` (start/stop/dock/zone), `robot.mower`.
- Energy: `energy.meter` (kwh), `energy.power` (w), `energy.tariff`, `energy.forecast`.
- Custom/vendor: `ext.*` namespace allowed; must document in registry.

## Capability Payload Shapes (starter set)
- `core.on_off`
  - State: `{ "state": "on" | "off" }`
  - Commands: `set` with `{ "state": "on" | "off", "transition_ms"?: number }`
- `core.level`
  - State: `{ "brightness": number }` (typical 1-254 range)
  - Commands: `set_level` with `{ "level": number, "transition_ms"?: number }`
- `core.color_xy`
  - State: `{ "x": number, "y": number }`
  - Commands: `set` with `{ "x": number, "y": number, "transition_ms"?: number }`
- `env.temperature`
  - State: `{ "value": number, "unit": "C" | "F" }`
- `core.power`
  - State: `{ "watts": number }`
- `energy.meter`
  - State: `{ "kwh": number, "interval_sec"?: number }`
- `media.control`
  - Commands: `play`, `pause`, `stop`, `seek` `{ "position_ms": number }`
- `camera.stream`
  - State: `{ "url": "rtsp://..." | "webrtc://..." }`
- `camera.detection`
  - Events: `{ "event": "object_detected", "data": { "class": string, "confidence": number, "bbox": [number, number, number, number] } }`
- `robot.vacuum`
  - Commands: `start`, `stop`, `dock`, `clean_zone` `{ "zone": string }`

## Command/State Patterns
- Booleans: `{"state": "on"|"off"}` or `true/false` (adapter normalizes).
- Numeric: include `unit` in metadata; payloads are raw numbers.
- Enums: strings (`mode`: `cool/heat/auto/dry`, `fan_speed`: `low/med/high/auto`).
- Arrays/objects: allowed for complex capabilities (scenes, zones, maps) but document in registry.

## Metadata vs State vs Events (retention rules)
- Metadata: retained; mostly static (model/manufacturer/capabilities) with optional user overrides (`user_meta`).
- State: retained latest value per capability/endpoint.
- Events: never retained; consumers process in real time.

## Pairing Flow (uniform)
- Start: publish pairing_command `action=start` with protocol + timeout + metadata.
- Progress: adapters emit `pairing_progress` stages (`listening`, `device_joined`, `interviewing`, `interview_complete`, `commissioning`, `finalizing`, `completed`, `failed`, `timeout`, `stopped`).
- Stop: publish pairing_command `action=stop` for protocol.
- Config: `/api/hdp/pairing-config` and/or retained `homenavi/hdp/pairing/config/{protocol}` to describe UI hints (instructions, default timeout, supports_interview, notes).

## Transport Rules
- MQTT/NATS: primary; retain last metadata/state where appropriate; commands and command_results are non-retained.
- REST/WS bridges: must translate to/from HDP envelopes; no ad-hoc shapes. All device data above device-hub (API GW → frontend, future rule engine/persistence/AI) must remain HDP-shaped.
- Versioning: `schema` must be `hdp.v1`; breaking changes require `v2` and dual-publish period.

## Adapter Responsibilities
- Subscribe to protocol-native topics/SDKs; publish HDP state/events/metadata (with endpoint when applicable).
- Subscribe to `homenavi/hdp/device/command/#` and translate to protocol commands; emit `command_result` on completion/failure.
- Handle pairing_command for its protocol; emit pairing_progress.
- Send hello/status heartbeats.
- Normalize units/types; drop protocol quirks from upstream services.

## Core Service Responsibilities (device-hub and above)
- Never parse protocol-native payloads; consume HDP only.
- Persist metadata/state using HDP schema; retain metadata/state in broker for bootstrap.
- Expose REST/WS for UI in HDP shape; no ad-hoc DTOs between device-hub → API gateway → frontend (and future rule engine/persistence/AI services).
- Route commands by publishing HDP command envelopes; expect command_result responses; do not talk to adapters directly.
- Maintain registry of capabilities and vendor extensions.

## Registry & Extensibility
- Keep a registry of capabilities (id, attributes, commands, units, ranges, preferred widgets, version).
- Vendors/extensions use `ext_vendor` namespaces; must document shape.
- New capabilities must be backward-compatible or versioned.

## Security & Auth
- Commands and pairing require auth; prefer signed tokens on MQTT (WS headers/Cookie already used today).
- Adapters authenticate to broker with adapter credentials; do not rely on anonymous publish.

## Migration Notes (current codebase gaps)
- Device bootstrap still uses REST; add HDP retained metadata/state topics and subscribe in frontend/device-hub; REST/WS must pass through HDP shapes only.
- Commands currently REST → internal MQTT; publish HDP command envelopes and deprecate protocol-specific command topics; add command_result responses.
- Pairing commands should move to `homenavi/hdp/pairing/command/{protocol}` (current `/api/hdp/pairings` is a bridge).
- Add retained pairing-config topics alongside the REST endpoint.
- Expand `protocolSupportsInterviewTracking` to cover thread/matter when adapters emit progress.

## Recommended Next Steps
1) Implement HDP command/topic bridge in device-hub; have adapters listen and act.
2) Publish retained metadata/state on HDP topics; let UI/rules subscribe instead of REST bootstrap.
3) Define registry JSON for capabilities (doc + machine-readable) and enforce validation at device-hub ingress.
4) Extend adapters (zigbee, thread, matter placeholders) to emit HDP envelopes for all state/events and to consume HDP commands.
5) Add tests that feed HDP messages end-to-end (adapter → device-hub → UI) for zigbee/thread/matter/camera/energy samples.
