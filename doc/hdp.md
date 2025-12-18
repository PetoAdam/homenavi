# HomeNavi Device Protocol (HDP)

HDP is HomeNavi’s internal, protocol-agnostic contract. Adapters translate protocol-native payloads (Zigbee2MQTT, Thread, etc.) into HDP messages. Everything above adapters (device-hub, history-service, api-gateway, frontend) speaks HDP only.

This document describes the **current HDP v1 contract implemented in this repository**.

## Versioning

- `schema`: `hdp.v1`
- `type`: message type (see below)

## Device Identity

### Canonical `device_id`

The canonical HDP device identifier is:

`protocol/<external>`

Examples:

- `zigbee/0x0017880104abcd`
- `thread/abcd1234`

Notes:

- `external` may itself contain `/`. HDP treats `device_id` as an opaque string.
- **MQTT topics** in this repo append `device_id` directly after the prefix (so a `device_id` with `/` will create deeper topic paths; this is expected).
- **HTTP URLs** that embed `device_id` in a path must allow `/` (or percent-encode it when it must be a single segment).

## MQTT Topics

All HDP traffic uses the `homenavi/hdp/` namespace.

### Device streams

- State (retained): `homenavi/hdp/device/state/<device_id>`
- Metadata (retained): `homenavi/hdp/device/metadata/<device_id>`
- Events (not retained): `homenavi/hdp/device/event/<device_id>`
- Commands (not retained): `homenavi/hdp/device/command/<device_id>`
- Command results (not retained): `homenavi/hdp/device/command_result/<device_id>`

### Pairing

- Pairing commands (not retained): `homenavi/hdp/pairing/command/<protocol>`
- Pairing progress (not retained): `homenavi/hdp/pairing/progress/<protocol>`

### Adapter presence

- Hello (not retained): `homenavi/hdp/adapter/hello`
- Status (retained): `homenavi/hdp/adapter/status/<adapter_id>`

## Envelopes

HDP messages are JSON objects with a small envelope. Fields vary by `type`.

Common fields:

- `schema` (string): `hdp.v1`
- `type` (string)
- `ts` (number): unix ms producer timestamp

### `state`

Topic: `homenavi/hdp/device/state/<device_id>` (retained)

```json
{
  "schema": "hdp.v1",
  "type": "state",
  "device_id": "zigbee/0x0017880104abcd",
  "ts": 1734114123123,
  "state": {
    "temperature": 21.4,
    "battery": 97
  },
  "corr": "optional-correlation-id"
}
```

Notes:

- `state` is a flat-ish JSON object of key/value pairs. The frontend extracts metrics from these keys.
- If present, `corr` links a state update to a prior command.

### `metadata`

Topic: `homenavi/hdp/device/metadata/<device_id>` (retained)

```json
{
  "schema": "hdp.v1",
  "type": "metadata",
  "device_id": "zigbee/0x0017880104abcd",
  "protocol": "zigbee",
  "name": "Hallway Light",
  "manufacturer": "Philips",
  "model": "LWB010",
  "description": "optional",
  "icon": "lightbulb",
  "online": true,
  "last_seen": "optional",
  "ts": 1734114123123
}
```

### `event`

Topic: `homenavi/hdp/device/event/<device_id>` (not retained)

```json
{
  "schema": "hdp.v1",
  "type": "event",
  "device_id": "zigbee/0x0017880104abcd",
  "event": "device_removed",
  "data": {},
  "ts": 1734114123123
}
```

Notes:

- `device_removed` is used to trigger cleanup (including clearing retained topics to prevent “ghost devices”).

### `command`

Topic: `homenavi/hdp/device/command/<device_id>` (not retained)

```json
{
  "schema": "hdp.v1",
  "type": "command",
  "device_id": "zigbee/0x0017880104abcd",
  "command": "set_state",
  "args": {
    "state": "ON",
    "brightness": 80
  },
  "corr": "cmd-1734114123123-123",
  "ts": 1734114123123
}
```

Notes:

- Current adapters primarily implement `command: "set_state"` with an arbitrary `args` map.
- Some producers may still include `correlation_id`; consumers should prefer `corr` but accept either.

### `command_result`

Topic: `homenavi/hdp/device/command_result/<device_id>` (not retained)

```json
{
  "schema": "hdp.v1",
  "type": "command_result",
  "device_id": "zigbee/0x0017880104abcd",
  "corr": "cmd-1734114123123-123",
  "success": true,
  "status": "accepted",
  "error": "optional error message",
  "ts": 1734114123123
}
```

## Pairing

### `pairing_command`

Topic: `homenavi/hdp/pairing/command/<protocol>`

```json
{
  "schema": "hdp.v1",
  "type": "pairing_command",
  "protocol": "zigbee",
  "action": "start",
  "timeout_sec": 60,
  "ts": 1734114123123
}
```

### `pairing_progress`

Topic: `homenavi/hdp/pairing/progress/<protocol>`

```json
{
  "schema": "hdp.v1",
  "type": "pairing_progress",
  "protocol": "zigbee",
  "stage": "interviewing",
  "status": "in_progress",
  "external_id": "0x0017880104abcd",
  "device_id": "zigbee/0x0017880104abcd",
  "friendly_name": "optional",
  "ts": 1734114123123
}
```

Notes:

- Pairing “defaults” are not owned by core services; **adapters advertise pairing UI config** via hello/status (see below).
- Device-hub also emits HDP pairing progress frames to the same progress topic (with `origin: "device-hub"`) and ignores its own frames to avoid loops.

## Adapter Hello / Status

Adapters announce themselves and advertise capabilities (including pairing UX/config) via:

- `homenavi/hdp/adapter/hello` (non-retained)
- `homenavi/hdp/adapter/status/<adapter_id>` (retained)

Example `hello`:

```json
{
  "schema": "hdp.v1",
  "type": "hello",
  "adapter_id": "zigbee-adapter",
  "protocol": "zigbee",
  "version": "0.0.0",
  "hdp_version": "1.0",
  "features": {
    "supports_pairing": true,
    "supports_interview": true
  },
  "pairing": {
    "label": "Zigbee (Zigbee2MQTT)",
    "supported": true,
    "supports_interview": true,
    "default_timeout_sec": 60,
    "instructions": ["..."],
    "cta_label": "Start Zigbee pairing"
  },
  "ts": 1734114123123
}
```

Device-hub stores these frames and exposes pairing config to the UI via its HTTP API.
