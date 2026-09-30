# Homenavi Next Fixes Implementation Plan

## Purpose

This plan covers the next reliability and product-UX improvements for device
inventory, Zigbee recovery actions, and the automation builder.

The work should be delivered in the order below. Each phase has an explicit
contract and test boundary so that the automation work does not turn into an
unbounded visual-editor rewrite.

## Phase 1: Make Device Inventory Mutations Immediately Consistent

### Problem

The Devices page merges two sources of truth:

- HDP devices and live state from `useDeviceHubDevices`.
- Rooms, ERS devices, groups, and metadata from `useErsInventory`.

Adding one or many devices through `AddDeviceModal`, or deleting a device from
the device details flow, can complete successfully while either merged source
waits for its next websocket event or polling cycle. The visible list is then
stale.

### Design

Create one parent-owned inventory mutation callback in the Devices page. It must
perform an immediate, debounced refresh of both sources after any successful
inventory mutation:

1. Refresh Device Hub metadata and state.
2. Refresh ERS inventory.
3. Clear pending pairing summaries only after both refreshes settle.
4. Surface a retryable snackbar if either refresh fails, while retaining the
   successful mutation result.

Use a 250-500 ms trailing debounce for a multi-device pairing session, so that
five devices completing within one interview window cause one merged refresh.
Do not use browser-only custom events as the primary contract.

### Touchpoints

- `frontend/src/components/Devices/Devices.jsx` or the page-level owner of the
  device inventory hooks.
- `frontend/src/components/Devices/AddDeviceModal.jsx` and
  `frontend/src/components/Devices/hooks/useDevicePairingMutations.js`.
- `frontend/src/components/Devices/DeviceDetails.jsx` and the delete mutation
  owner.
- `frontend/src/hooks/useDeviceHubDevices.js`.
- `frontend/src/hooks/useErsInventory.js`.

### Acceptance Criteria

- A successfully added single device appears without a manual browser refresh.
- Multiple paired devices produce one bounded refresh after the final completion
  event.
- A deleted device disappears immediately from the list, map palette, and group
  selectors after the mutation succeeds.
- A websocket delivery gap cannot leave the list stale after a successful API
  response.
- Component tests cover add, multi-add, remove, one-source refresh failure, and
  retry.

## Phase 2: Repair and Instrument Zigbee Reinterview/Reconfigure

### Current State

The command path already exists:

- The device hub accepts a reconfigure request and owns the exclusive command
  lock in `device-hub/internal/http/devices.go`.
- Zigbee adapter commands are translated in
  `zigbee-adapter/internal/proto/zigbee/commands.go`.
- Zigbee2MQTT currently supports `interview` and `force_interview`; unsupported
  modes must be rejected rather than silently mapped.
- Completion is driven by a `device_interview` bridge event and a correlation
  entry in the adapter.

The remaining failure must be diagnosed from the real HTTP response, outgoing
MQTT topic/payload, bridge event, friendly-name mapping, and lock lifecycle. Do
not label every timeout as a device-offline problem.

### Implementation

1. Define a versioned reconfigure capability returned by each adapter:
   supported modes, required arguments, expected timeout, and whether a bridge
   acknowledgement is available.
2. Make the Devices UI render actions exclusively from that capability. For
   Zigbee, expose `Reinterview` and `Force reinterview`; do not expose a generic
   `reconfigure` mode that the adapter cannot execute.
3. Make the command lifecycle observable: queued, bridge accepted,
   interviewing, completed, failed, or timed out. Persist the correlation ID
   long enough for a reconnecting client to recover the status.
4. When a friendly-name lookup cannot resolve an external ID, refresh bridge
   devices once and retry resolution before returning a typed mapping error.
5. On terminal success, run the Phase 1 inventory refresh. On terminal failure,
   show the adapter-provided error and a contextual recovery action.
6. Add structured logs and metrics for each terminal status, command latency,
   mapping miss, lock conflict, and timeout.

### Tests

- Device hub contract tests for each advertised and unsupported mode.
- Zigbee adapter tests for topic/payload generation, correlation recovery,
  friendly-name refresh/retry, success, adapter failure, and timeout.
- Frontend tests for capability-based actions, disabled conflicting commands,
  progress updates, and retry messaging.

## Phase 3: Capability-Driven Automation Actions and Device-State Triggers

### Problem

`ActionSendCommandEditor` currently presents fixed fields such as state,
brightness, transition, and color mode. `TriggerEditor` derives state-key
choices from a device's current payload rather than its capability schema. This
makes valid controls undiscoverable and lets the UI construct invalid commands.

### Capability Contract

Create a frontend capability view model from Device Hub metadata:

- `id` and property path.
- type: binary, numeric, enum, string, object, or unsupported.
- access: read/write.
- unit, range, step, enum values, and display label.
- target compatibility for a device, group, or selector.

The model must preserve the raw protocol payload but normalize it for UI use in
one place. A group control is only editable when all selected devices share a
compatible writable capability; otherwise show an explicit compatibility state.

### Action Node UX

1. Select target first: device, group, or selector.
2. Fetch/resolve compatible writable capabilities for the target.
3. Render typed controls: toggle for binary, stepper/slider for numeric range,
   select for enum, and JSON only for object/custom capabilities.
4. Keep a clearly labelled advanced JSON command mode, but validate it against
   the selected target capability when a target is known.
5. Replace hardcoded transition/color assumptions with capabilities supplied by
   the selected integration or device.

### Device-State Trigger UX

1. Select the target, then a readable capability, then a compatible operator.
2. Show only valid operators: equality for enums/binary, numeric comparisons
   for numeric values, and exists/changed for all readable state.
3. Format a typed comparison input with the capability unit and range.
4. Support selector/group targets using an explicit aggregation policy:
   any, all, or each device. Do not silently infer it.
5. Add optional debounce and cooldown controls near the condition rather than
   burying them in raw node JSON.

### Backend Work

- Keep `action.send_command` backward compatible, but validate known
  capability-targeted commands before dispatch.
- Extend `trigger.device_state` validation with target aggregation and typed
  operands while accepting existing literal workflow definitions.
- Treat catalog metadata as advisory. The device hub remains the execution-time
  authority for access checks and protocol translation.

### Tests

- Capability view-model unit tests for every supported type.
- Editor tests for valid controls, incompatible groups, stale metadata, and
  advanced JSON fallback.
- Engine and Device Hub contract tests proving invalid writes are rejected
  before adapter dispatch.

## Phase 4: Structured Control Flow for For and If

### Principle

Do not implement loops by permitting arbitrary graph cycles. Preserve a
top-level directed acyclic workflow graph and introduce structured composite
nodes with explicit internal scopes. This gives the editor the Lego-style visual
containment requested while keeping validation, execution, and observability
tractable.

### Workflow Definition v2

Introduce a versioned definition alongside the current flat format. A composite
node owns scoped child blocks:

```json
{
  "kind": "logic.for",
  "data": { "iterator": "item", "count": 3 },
  "body": { "nodes": [], "edges": [] },
  "next": "after-loop-node"
}
```

An `logic.if` node owns `when_true` and `when_false` blocks plus its continuation
edge. In the canvas, the parent node stays on the left and exposes an embedded
body area: one vertical body for `for`, and side-by-side True/False columns for
`if`. A branch may contain another composite node.

### Execution and Limits

1. Build a recursive compiler from the persisted definition to a validated
   execution plan. Each block has its own node-ID namespace and continuation.
2. Maintain scoped execution variables, beginning with a loop iterator/index.
   Variables are read-only outside their owning block.
3. Enforce limits at validation time: maximum nesting depth of 3, maximum 100
   nodes per workflow, maximum 1,000 loop iterations per run, execution timeout,
   and a clear fan-out limit for parallel branches.
4. Preserve the current positional-edge behavior only for v1 workflows. New
   workflows use explicit `body`, `true`, `false`, and `next` ports; do not rely
   on edge ordering.
5. Record scoped node paths in run events, logs, traces, and the run-history UI
   so a failed nested action is diagnosable.

### Conditions and Device Comparisons

Replace the flat `{ path, op, value }` condition with typed operands:

```json
{
  "left": { "kind": "device_state", "device_id": "...", "key": "temperature" },
  "op": "gt",
  "right": { "kind": "device_state", "device_id": "...", "key": "setpoint" }
}
```

Supported operand kinds are literal, triggering-event field, device state, and
scoped variable. Resolve device-state operands into a run-consistent snapshot,
apply capability-aware coercion, and return structured evaluation errors rather
than silently comparing incompatible values.

### Migration

- Continue executing v1 workflows unchanged.
- Provide a one-way v1-to-v2 conversion only where an existing graph maps
  unambiguously to branch/body structure.
- Require explicit user confirmation for ambiguous graphs.
- Version the public workflow API and document the compatibility window.

## Phase 5: Sunrise and Sunset Schedule Triggers

### Recommendation

Use a solar calendar owned by the weather service, not a synchronous weather API
call on every automation firing. A daily refresh is necessary but not sufficient
on its own: the schedule needs a durable, time-zone-aware calendar and an
idempotent way to re-register workflow triggers when the next solar event moves.

### Architecture

1. Add a weather-service solar endpoint backed by an astronomical calculation or
   the selected weather provider. Store `sunrise`, `sunset`, timezone, location,
   source, and calculation date for at least the next 48 hours.
2. Refresh the calendar daily and whenever the configured home location or
   timezone changes. Use an idempotent job with metrics, retry/backoff, and an
   alert for stale calendar data.
3. Publish a durable `solar_calendar_updated` event after each successful
   refresh.
4. Add `trigger.solar` to automation definitions with `event` (sunrise/sunset),
   signed offset in minutes, location scope, and fallback policy.
5. Automation service subscribes to calendar updates and atomically replaces
   only the affected scheduled entries. It must also reconcile schedules at
   startup, as it does for cron triggers.
6. At fire time, use the persisted calculated instant. Never block execution on
   a live provider call. If the calendar is stale, follow the workflow's
   configured policy: skip with an observable failure or use the last calculated
   occurrence within a bounded grace period.

### UI

Add Sunrise and Sunset choices to the schedule trigger builder with an offset
stepper and a location/timezone summary. Keep raw cron as an advanced alternative
for fixed schedules, not as the representation of solar schedules.

### Tests

- DST transition, polar day/night, timezone change, stale calendar, and offset
  tests.
- Multi-replica schedule reconciliation and durable-cooldown tests.
- End-to-end test proving a calendar update changes the next trigger exactly
  once.

## Delivery Sequence

1. Device mutation refresh and Zigbee command observability.
2. Capability view model, action editor, and state-trigger editor.
3. Workflow definition v2, beginning with `if` true/false branches.
4. Scoped `for` blocks and typed operands/device-state comparisons.
5. Solar calendar service and solar triggers.

No phase should be merged without its API contract tests, UI tests, and
observability coverage for its new asynchronous behavior.