# Automation Engine v2 Implementation Spec

## Scope

This document specifies the implementation for four related automation engine upgrades in Homenavi:

1. ERS groups must behave as first-class targets for `action.send_command` and `trigger.device_state`.
2. Interactive device command and device status trigger editors must be derived from HDP metadata and ERS target resolution, not from hardcoded command and state presets.
3. `logic.for` must become a real loop primitive with explicit body and after branches plus loop variables.
4. `logic.if` must expose explicit `then` and `else` outputs in the graph model and editor.
5. Automations must also be storable as code, with a conversion path into the graph editor when the automation was not originally authored in graph form.

This spec covers persisted workflow schema changes, runtime behavior, API contracts, migration behavior, rollout order, and a file-by-file implementation breakdown.

## Goals

- Keep ERS as the canonical owner of groups, rooms, tags, and selector resolution.
- Keep HDP and device-hub as the canonical owner of device metadata, capabilities, inputs, state, and command semantics.
- Keep the persisted workflow format backwards-compatible through migration.
- Allow a workflow to have a code-first source of truth while still remaining editable in the graph UI through deterministic conversion.
- Remove hardcoded automation editor assumptions around `state`, `brightness`, `transition_ms`, `color_temp`, `hue`, and `saturation`.
- Reuse existing capability normalization patterns already present in the frontend and adapters.

## Non-goals

- Do not add arbitrary graph cycles.
- Do not change the overall workflow storage location or repository model.
- Do not replace ERS selector resolution with a second inventory system.
- Do not require adapters to implement a new command protocol before phase 1 ships.
- Do not require the first code format to support every future automation feature before dual-format storage ships.

## Current Anchors In The Repo

Backend surfaces already in place:

- `automation-service/internal/engine/definition.go`
  - supports selector targets for `trigger.device_state` and `action.send_command`
  - already contains `logic.if` and `logic.for` node types
- `automation-service/internal/engine/triggers.go`
  - already resolves selector targets through ERS and matches device state triggers
- `automation-service/internal/engine/execution.go`
  - already executes `logic.if` with two positional branches and `logic.for` with two positional branches
- `entity-registry-service/internal/infra/db/selectors.go`
  - already resolves `group:<slug>` selectors to HDP device ids

Frontend surfaces that must change:

- `frontend/src/components/Automation/definition.js`
  - hardcodes editor defaults and builder serialization for device commands and device triggers
- `frontend/src/components/Automation/components/nodeEditors/ActionSendCommandEditor.jsx`
  - hardcodes `set_state` interactive mode
- `frontend/src/components/Automation/components/nodeEditors/TriggerEditor.jsx`
  - hardcodes single-device trigger editing and uses current state keys only
- `frontend/src/components/Automation/components/AutomationCanvas.jsx`
  - exposes only one visible output port even for branch nodes

Existing reusable capability and group-control logic:

- `frontend/src/hooks/useDeviceHubDevices.js`
  - already hydrates device metadata, capabilities, inputs, configuration readiness, and state
- `frontend/src/utils/groupControls.js`
  - already computes common writable controls and common state fields across a group of devices

Persistence surfaces already in place:

- `automation-service/internal/infra/db/models.go`
  - currently stores only `Workflow.Definition` as JSONB
- `automation-service/internal/http/workflows.go`
  - currently accepts and validates only graph JSON in the `definition` field

## Architectural Decisions

### 1. Persist explicit edge ports

Branch semantics must move from edge ordering to named output ports.

### 2. Keep `version: "automation"`

The top-level workflow `version` remains `"automation"`.

Add `schema_revision` to allow safe migration without breaking existing readers.

### 3. Keep canonical runtime actions backend-friendly

Interactive editor state may be stored under `data.ui`, but canonical runtime fields must continue to compile to simple backend payloads.

### 4. Reuse HDP metadata and normalized device inputs

Capability-driven editors must use HDP metadata and normalized device `inputs` and `capabilities` instead of introducing a separate automation-only command schema.

### 5. Resolve selectors through ERS, derive capability shape through device-hub

ERS answers "which devices are targeted".

device-hub answers "what controls and state fields are available for those devices".

### 6. Introduce a canonical intermediate representation for automation source formats

The graph JSON remains the runtime execution format, but the system must support two authoring formats:

- graph JSON
- code source

The conversion pipeline must always produce a normalized canonical graph definition before validation and execution.

### 7. Store both canonical graph and optional original code source

The runtime engine should continue loading canonical graph definitions.

If a workflow is authored as code, the original code source must also be stored so users can round-trip between code and graph instead of losing authorship context.

### 8. Ship a deterministic code format before shipping a free-form language

The first code source format should be a structured Homenavi automation code format, referred to in this spec as `hnac.v1`.

Recommended first encoding:

- YAML text for human authoring
- JSON-equivalent schema for validation and conversion

This avoids trying to infer graph semantics from arbitrary JavaScript, Python, or shell code.

## Persisted Workflow Schema Changes

## Workflow persistence model

Current persistence model:

- workflow row stores only `definition` JSONB

New persistence model:

- workflow row stores canonical `definition_graph` JSONB
- workflow row may also store `source_kind`, `source_format`, and `source_code`

### Current database model

```go
type Workflow struct {
    ID         uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
    Name       string         `gorm:"not null" json:"name"`
    Enabled    bool           `gorm:"not null;default:false" json:"enabled"`
    Definition datatypes.JSON `gorm:"type:jsonb;not null" json:"definition"`
    CreatedBy  string         `gorm:"not null" json:"created_by"`
    CreatedAt  time.Time      `json:"created_at"`
    UpdatedAt  time.Time      `json:"updated_at"`
}
```

### New database model

```go
type Workflow struct {
    ID             uuid.UUID      `gorm:"type:uuid;primaryKey" json:"id"`
    Name           string         `gorm:"not null" json:"name"`
    Enabled        bool           `gorm:"not null;default:false" json:"enabled"`
    Definition     datatypes.JSON `gorm:"type:jsonb;not null" json:"definition"`
    SourceKind     string         `gorm:"not null;default:'graph'" json:"source_kind"`
    SourceFormat   string         `gorm:"not null;default:'graph-json'" json:"source_format"`
    SourceCode     string         `gorm:"type:text" json:"source_code,omitempty"`
    SourceRevision int            `gorm:"not null;default:1" json:"source_revision"`
    CreatedBy      string         `gorm:"not null" json:"created_by"`
    CreatedAt      time.Time      `json:"created_at"`
    UpdatedAt      time.Time      `json:"updated_at"`
}
```

### Persistence rules

- `Definition` remains the canonical graph definition used by runtime.
- `SourceKind` values:
  - `graph`
  - `code`
- `SourceFormat` values in the first rollout:
  - `graph-json`
  - `hnac.v1.yaml`
  - `hnac.v1.json`
- `SourceCode` is empty for `graph` workflows.
- `SourceCode` stores the original code text for `code` workflows.
- `SourceRevision` versions the source format independently from `schema_revision` inside the graph definition.

## Top-level `Definition`

Current shape:

```json
{
  "version": "automation",
  "nodes": [],
  "edges": []
}
```

New shape:

```json
{
  "version": "automation",
  "schema_revision": 2,
  "nodes": [],
  "edges": []
}
```

Rules:

- Missing `schema_revision` means revision `1`.
- Saving from the updated editor always writes `schema_revision: 2`.
- Code-authored workflows are compiled to this same canonical graph shape before runtime validation.

### Go struct change

File:

- `automation-service/internal/engine/definition.go`

Change:

```go
type Definition struct {
    Version        string    `json:"version"`
    SchemaRevision int       `json:"schema_revision,omitempty"`
    Nodes          []NodeDef `json:"nodes"`
    Edges          []EdgeDef `json:"edges"`
}
```

## `EdgeDef`

Current shape:

```json
{ "from": "if_1", "to": "action_1" }
```

New shape:

```json
{ "from": "if_1", "to": "action_1", "from_port": "then" }
```

Allowed `from_port` values:

- `default`
- `then`
- `else`
- `body`
- `after`

Rules:

- Non-branch nodes use `default`.
- `logic.if` may emit at most one `then` edge and at most one `else` edge.
- `logic.for` may emit at most one `body` edge and at most one `after` edge.
- Revision 1 workflows migrate positional edges to ports using current ordering behavior:
  - `logic.if`: first outgoing edge becomes `then`, second becomes `else`
  - `logic.for`: first outgoing edge becomes `body`, second becomes `after`
  - other nodes: all outgoing edges become `default`

### Go struct change

```go
type EdgeDef struct {
    From     string `json:"from"`
    To       string `json:"to"`
    FromPort string `json:"from_port,omitempty"`
}
```

## Code source schema: `hnac.v1`

The first code-based authoring format is a structured text format that compiles directly to the graph definition.

Canonical YAML example:

```yaml
version: hnac.v1
workflow:
  name: Kitchen Motion Lights
  trigger:
    kind: device_state
    targets:
      type: selector
      selector: group:kitchen-sensors
    path: state.motion
    op: eq
    value: true
  steps:
    - if:
        path: trigger.state.motion
        op: eq
        value: true
      then:
        - send_command:
            targets:
              type: selector
              selector: group:kitchen-spots
            command: set_state
            args:
              state: ON
      else:
        - send_command:
            targets:
              type: selector
              selector: group:kitchen-spots
            command: set_state
            args:
              state: OFF
```

### `hnac.v1` principles

- structured, not arbitrary code execution
- text-first and versioned
- deterministic conversion into canonical graph JSON
- capable of expressing explicit branching and loops

### `hnac.v1` conversion rules

- every trigger compiles to a trigger node
- every step compiles to one or more graph nodes
- `if.then` compiles to a `logic.if` node with `then` edges
- `if.else` compiles to a `logic.if` node with `else` edges
- `for.body` compiles to a `logic.for` node with `body` edges
- implicit sequence order compiles to `default` edges between nodes

### `hnac.v1` validation rules

- converter rejects ambiguous structures that cannot be represented in the graph model
- converter must produce stable node ids when possible during a single conversion pass
- converter must annotate generated nodes with source mapping metadata in editor-only `ui` fields when useful

## `TriggerDeviceState`

Current shape:

```json
{
  "targets": { "type": "device", "ids": ["zigbee/abc"] },
  "key": "motion",
  "op": "eq",
  "value": true,
  "cooldown_sec": 2,
  "ignore_retained": true
}
```

New canonical shape:

```json
{
  "targets": { "type": "selector", "selector": "group:kitchen-spots" },
  "path": "state.motion",
  "op": "eq",
  "value": true,
  "match_mode": "any",
  "cooldown_sec": 2,
  "ignore_retained": true
}
```

Rules:

- `path` replaces `key` as the canonical field selector.
- `key` remains accepted for migration only.
- `match_mode` values:
  - `any`: fire when any targeted device satisfies the trigger
  - `all`: fire when all currently resolved targeted devices satisfy the trigger
- `path` uses dot notation rooted at the trigger event payload.
- Device state trigger interactive mode should write paths using `state.<property>`.

Migration:

- If `path` is empty and `key` is set, normalize to `path = "state." + key`.

### Go struct change

```go
type TriggerDeviceState struct {
    Targets        NodeTargets     `json:"targets"`
    Path           string          `json:"path,omitempty"`
    Key            string          `json:"key,omitempty"`
    Op             string          `json:"op,omitempty"`
    Value          json.RawMessage `json:"value,omitempty"`
    MatchMode      string          `json:"match_mode,omitempty"`
    CooldownSec    int             `json:"cooldown_sec,omitempty"`
    IgnoreRetained bool            `json:"ignore_retained,omitempty"`
}
```

## `ActionSendCommand`

Canonical runtime payload remains simple:

```json
{
  "targets": { "type": "selector", "selector": "group:kitchen-spots" },
  "command": "set_state",
  "args": {
    "brightness": 120
  },
  "wait_for_result": false,
  "result_timeout_sec": 15
}
```

Editor-only interactive metadata is persisted under `ui`:

```json
{
  "targets": { "type": "selector", "selector": "group:kitchen-spots" },
  "command": "set_state",
  "args": { "brightness": 120 },
  "wait_for_result": false,
  "result_timeout_sec": 15,
  "ui": {
    "mode": "interactive",
    "command_source": "capability_input",
    "input_id": "brightness",
    "property": "brightness",
    "value_type": "number",
    "value_number": 120,
    "value_bool": false,
    "value_string": "",
    "value_color": "#FFFFFF",
    "value_json": ""
  }
}
```

Rules:

- The backend ignores `ui`.
- The frontend compiles `ui` to canonical `command` and `args` before save.
- Custom JSON mode stays supported.
- `wait_for_result` remains limited to single-device targets.

## `LogicIf`

Payload does not need a large runtime change, but the graph contract changes because branches are explicit.

Canonical payload stays:

```json
{
  "path": "vars.i",
  "op": "lt",
  "value": 3
}
```

Graph contract:

- `then` edge is explicit.
- `else` edge is explicit.

## `LogicFor`

Current shape:

```json
{ "count": 3 }
```

New shape:

```json
{
  "mode": "count",
  "count": 3,
  "variable": "i"
}
```

Range form:

```json
{
  "mode": "range",
  "variable": "i",
  "from": 0,
  "to": 10,
  "step": 1
}
```

Rules:

- Supported modes in this rollout:
  - `count`
  - `range`
- `variable` defaults to `i`.
- `count` must be `>= 0`.
- `step` must not be `0`.
- `body` edge is explicit.
- `after` edge is explicit.

Migration:

- Revision 1 `logic.for { count: N }` becomes `mode: "count"`, `count: N`, `variable: "i"`.

### Go struct change

```go
type LogicFor struct {
    Mode     string  `json:"mode,omitempty"`
    Count    int     `json:"count,omitempty"`
    Variable string  `json:"variable,omitempty"`
    From     float64 `json:"from,omitempty"`
    To       float64 `json:"to,omitempty"`
    Step     float64 `json:"step,omitempty"`
}
```

## Runtime Execution Model Changes

## New internal execution context

Add an internal context object in automation-service runtime.

Proposed internal shape:

```go
type ExecutionContext struct {
    Trigger map[string]any
    Vars    map[string]any
}
```

Rules:

- `Trigger` contains the original trigger event.
- `Vars` contains loop variables and future workflow variables.
- `logic.if` reads from the full execution context, not only the trigger event.
- `logic.for` writes `Vars[variable]` per iteration.

## `logic.if` evaluation

`evalIf` must support paths rooted at:

- `trigger.*`
- `vars.*`
- legacy bare paths, which are interpreted as:
  - `state.foo` if they came from old device-state behavior
  - direct trigger payload lookup otherwise

Recommended canonical behavior:

- new editor writes `trigger.state.motion`
- loop-aware editor writes `vars.i`

## `logic.for` execution

`logic.for` must:

- resolve its `body` and `after` ports explicitly
- create per-iteration loop variable values
- execute body serially for each iteration
- execute `after` once after the loop completes

Count example:

- `count = 3`
- `variable = "i"`
- iterations expose `vars.i = 0`, then `1`, then `2`

Range example:

- `from = 2`, `to = 6`, `step = 2`
- iterations expose `vars.i = 2`, then `4`, then `6`

## Group trigger semantics

`trigger.device_state` with selector targets must support `match_mode`.

Rules:

- `any`
  - fire when the current message device matches the selector and that device satisfies the condition
- `all`
  - fire only when all currently resolved devices satisfy the condition according to latest known state

Important implementation note:

- `all` requires latest-state tracking for targeted devices inside automation-service or a helper query source.
- To reduce phase 1 scope, `all` may be accepted in schema but shipped behind a feature flag until state-cache support lands.

## New API Contracts

## Workflow CRUD payloads

Current workflow payload:

```json
{
  "name": "My workflow",
  "enabled": false,
  "definition": {
    "version": "automation",
    "nodes": [],
    "edges": []
  }
}
```

New workflow create and update payload:

```json
{
  "name": "My workflow",
  "enabled": false,
  "definition": {
    "version": "automation",
    "schema_revision": 2,
    "nodes": [],
    "edges": []
  },
  "source": {
    "kind": "graph",
    "format": "graph-json",
    "code": ""
  }
}
```

Code-authored workflow payload:

```json
{
  "name": "Kitchen Motion Lights",
  "enabled": false,
  "source": {
    "kind": "code",
    "format": "hnac.v1.yaml",
    "code": "version: hnac.v1\nworkflow:\n  name: Kitchen Motion Lights\n  ..."
  }
}
```

Rules:

- graph create/update requests may provide `definition` and optional `source.kind = graph`
- code create/update requests may provide `source.kind = code` without `definition`
- backend compiles code source into canonical `definition` before storing
- response always returns canonical `definition`
- response also returns stored `source` metadata

## New conversion endpoints

### Convert code to graph

Endpoint:

`POST /api/automations/convert/code-to-graph`

Request:

```json
{
  "format": "hnac.v1.yaml",
  "code": "version: hnac.v1\nworkflow:\n  ...",
  "options": {
    "assign_layout": true,
    "preserve_source_map": true
  }
}
```

Response:

```json
{
  "source": {
    "kind": "code",
    "format": "hnac.v1.yaml"
  },
  "definition": {
    "version": "automation",
    "schema_revision": 2,
    "nodes": [],
    "edges": []
  },
  "warnings": [],
  "source_map": {
    "nodes": {
      "if_1": { "line": 8, "column": 7 }
    }
  }
}
```

### Convert graph to code

Endpoint:

`POST /api/automations/convert/graph-to-code`

Request:

```json
{
  "format": "hnac.v1.yaml",
  "definition": {
    "version": "automation",
    "schema_revision": 2,
    "nodes": [],
    "edges": []
  }
}
```

Response:

```json
{
  "format": "hnac.v1.yaml",
  "code": "version: hnac.v1\nworkflow:\n  ...",
  "warnings": []
}
```

### Conversion API rules

- conversion must be deterministic for the same input
- graph-to-code may reject graph constructs that are not representable in `hnac.v1`
- code-to-graph must return a valid canonical graph definition or a structured validation error
- these endpoints are used by both the UI and future CLI tooling

## ERS selector resolution

No breaking ERS API change is required.

Existing endpoint remains:

`POST /api/ers/selectors/resolve`

Request:

```json
{
  "selector": "group:kitchen-spots"
}
```

Response:

```json
{
  "hdp_external_ids": ["zigbee/abc", "zigbee/def"]
}
```

## New device-hub helper endpoint: target profile

Purpose:

- centralize capability and state-field normalization for automation editors
- avoid duplicating target-profile logic across multiple frontend components

Endpoint:

`POST /api/hdp/automation/target-profile`

Request:

```json
{
  "device_ids": ["zigbee/abc", "zigbee/def"],
  "include": {
    "devices": true,
    "shared_inputs": true,
    "shared_state_fields": true
  }
}
```

Response:

```json
{
  "devices": [
    {
      "device_id": "zigbee/abc",
      "online": true,
      "manufacturer": "Philips",
      "model": "LWB010",
      "capabilities": [
        {
          "id": "brightness",
          "name": "Brightness",
          "kind": "numeric",
          "property": "brightness",
          "value_type": "number",
          "access": { "read": true, "write": true, "event": false },
          "range": { "min": 0, "max": 254, "step": 1 }
        }
      ],
      "inputs": [
        {
          "id": "brightness",
          "label": "Brightness",
          "type": "slider",
          "capability_id": "brightness",
          "property": "brightness",
          "range": { "min": 0, "max": 254, "step": 1 },
          "options": [],
          "metadata": {}
        }
      ],
      "state_fields": [
        {
          "path": "state.brightness",
          "property": "brightness",
          "label": "Brightness",
          "value_type": "number",
          "unit": "",
          "range": { "min": 0, "max": 254, "step": 1 }
        }
      ]
    }
  ],
  "shared_inputs": [
    {
      "id": "brightness",
      "label": "Brightness",
      "type": "slider",
      "capability_id": "brightness",
      "property": "brightness",
      "range": { "min": 0, "max": 254, "step": 1 },
      "options": [],
      "metadata": {}
    }
  ],
  "shared_state_fields": [
    {
      "path": "state.brightness",
      "property": "brightness",
      "label": "Brightness",
      "value_type": "number",
      "unit": "",
      "range": { "min": 0, "max": 254, "step": 1 }
    }
  ],
  "warnings": []
}
```

Rules:

- `shared_inputs` contains only controls valid across all provided devices.
- `shared_state_fields` contains only readable state fields common across all provided devices.
- Empty arrays are valid and must not fail the request.

## Frontend target-profile resolution flow

Single device:

1. target already contains device id
2. call `POST /api/hdp/automation/target-profile` with one id

Group or other selector:

1. call `POST /api/ers/selectors/resolve`
2. call `POST /api/hdp/automation/target-profile` with resolved ids

Advanced selector UI behavior:

- if selector resolution fails, interactive mode is disabled and JSON mode remains available

## Graph editor conversion flow for code-authored workflows

When the user opens a workflow in the graph editor and `source.kind = code`:

1. load canonical `definition` from the workflow record
2. if `definition` is present and valid, render it directly
3. if `definition` is missing or stale, call `POST /api/automations/convert/code-to-graph`
4. render the returned graph definition in the graph editor
5. preserve `source.kind = code` and `source.code` unless the user explicitly chooses to switch source-of-truth to graph

Optional editor actions:

- `Open code`
- `Refresh graph from code`
- `Export graph to code`
- `Switch source of truth to graph`

## Source-of-truth switching rules

Opening a code-authored workflow in the graph editor must not silently discard the original code.

Recommended behavior:

1. code-authored workflow opens as a converted graph working copy
2. if the user makes no changes, original `source.kind = code` remains unchanged
3. if the user saves after visual edits, the UI prompts for one of these outcomes:
   - `Keep code as source of truth`
   - `Switch source of truth to graph`
4. `Keep code as source of truth` is allowed only when graph-to-code export for the current workflow is supported by the selected code format
5. if graph-to-code export is not yet supported for the changed workflow, only `Switch source of truth to graph` is allowed

Persistence result when switching source of truth:

- switch to graph:
  - update canonical `definition`
  - set `source_kind = graph`
  - set `source_format = graph-json`
  - write updated graph to `source_graph`
  - preserve previous code only in audit history if such history exists, not as active source
- keep code:
  - export the edited graph back to code
  - update `source_code`
  - keep `source_kind = code`
  - refresh canonical `definition` from exported code or from the edited canonical graph if export guarantees equivalence

## Frontend Editor Data Shapes

## Trigger editor `ui`

Persisted under `node.data.ui`:

```json
{
  "editor_mode": "interactive",
  "field_source": "shared_state_field",
  "selected_field": {
    "path": "state.motion",
    "property": "motion",
    "label": "Motion",
    "value_type": "boolean"
  },
  "value_mode": "typed",
  "value_type": "boolean",
  "value_bool": true,
  "value_number": "",
  "value_string": "",
  "value_json": "",
  "match_mode": "any"
}
```

Compilation rules:

- `selected_field.path` compiles to canonical `data.path`
- typed UI values compile to canonical `data.value`
- `ui.match_mode` compiles to canonical `data.match_mode`

## Action editor `ui`

Persisted under `node.data.ui`:

```json
{
  "editor_mode": "interactive",
  "command_source": "shared_input",
  "selected_input": {
    "id": "brightness",
    "property": "brightness",
    "label": "Brightness",
    "type": "slider",
    "value_type": "number"
  },
  "value_type": "number",
  "value_number": 120,
  "value_bool": false,
  "value_string": "",
  "value_color": "#FFFFFF",
  "value_json": ""
}
```

Compilation rules:

- selected input compiles to `command = "set_state"`
- selected input property becomes a key in `args`
- color, toggle, enum, number, and text values compile according to normalized input type

## Phased Rollout Plan

## Phase 0: Docs and type-safe migration scaffolding

Deliverables:

- add schema revision handling
- add edge port support in definitions
- add dual-format workflow persistence fields and migration scaffolding
- add documentation and migration tests

No user-facing behavior change is required in this phase.

## Phase 1: Explicit control-flow semantics

Deliverables:

- explicit `from_port` support in definitions
- if node uses `then` and `else`
- for node uses `body` and `after`
- canvas shows labeled branch ports
- revision 1 workflows migrate automatically

Acceptance criteria:

- saving any updated workflow writes `schema_revision: 2`
- edge ordering no longer determines branch semantics after save
- old workflows still execute correctly

## Phase 2: Code-first storage and conversion pipeline

Deliverables:

- add workflow `source_kind`, `source_format`, and `source_code` persistence
- add `hnac.v1` parser and compiler into canonical graph definitions
- add `code-to-graph` and `graph-to-code` conversion APIs
- allow code-authored workflows to open in the graph editor via conversion

Acceptance criteria:

- a workflow may be created from code without supplying graph JSON
- the backend stores canonical graph `definition` plus original code source
- the graph editor can open and render code-authored workflows
- non-representable graph constructs return a structured conversion error in graph-to-code

## Phase 3: Target profile API and frontend resolution

Deliverables:

- new device-hub target-profile endpoint
- frontend target-profile hook for device and selector targets
- shared inputs and shared state fields for groups and selectors

Acceptance criteria:

- a group target can return common writable controls
- a group target can return common state fields
- advanced selector targets can enable interactive mode when resolution succeeds

## Phase 4: Capability-driven device command editor

Deliverables:

- replace hardcoded `set_state` builder UI with metadata-driven input editors
- preserve JSON fallback mode
- support device, group, and selector targets consistently

Acceptance criteria:

- interactive command options come from HDP metadata and normalized inputs
- group commands only expose shared controls
- invalid `wait_for_result` combinations are still blocked

## Phase 5: Capability-driven device state trigger editor

Deliverables:

- replace current-state-key-only UI with metadata-driven state field editor
- add `path` and `match_mode`
- preserve JSON fallback mode

Acceptance criteria:

- device-state triggers can be built from shared fields for groups
- triggers serialize to canonical `path`, `op`, `value`, and `match_mode`

## Phase 6: Real loop semantics

Deliverables:

- add internal execution context with variables
- upgrade `logic.for` to `count` and `range` modes
- enable `logic.if` to evaluate `vars.*`

Acceptance criteria:

- `logic.for` can expose `vars.i`
- `logic.if` can branch on `vars.i`
- `after` branch runs once after loop completion

## Phase 7: Group runtime semantics hardening

Deliverables:

- add support for `match_mode = all`
- introduce latest-state tracking if required
- broaden tests for selector-targeted triggers and group actions

Acceptance criteria:

- `any` is stable for all selector targets
- `all` is either fully supported or explicitly feature-gated

## File-by-file Task Breakdown

## Phase 1 files

| File | Change |
| --- | --- |
| `automation-service/internal/engine/definition.go` | add `SchemaRevision`, `EdgeDef.FromPort`, migration normalization, validation for explicit ports, update `LogicFor` shape |
| `automation-service/internal/engine/execution.go` | replace positional branch lookup with port-based lookup, add loop context scaffolding |
| `frontend/src/components/Automation/definition.js` | persist `schema_revision`, emit `from_port`, migrate old edges on parse, update validation rules |
| `frontend/src/components/Automation/components/AutomationCanvas.jsx` | render separate output handles for `then`, `else`, `body`, and `after`; update connect semantics |
| `frontend/src/components/Automation/hooks/useAutomationConnectMode.js` | capture source port during connect mode and persist it on edges |
| `frontend/src/components/Automation/hooks/useAutomationCanvas.js` | support creation and movement of branch-aware nodes without losing port metadata |
| `frontend/src/components/Automation/automationCanvasSelectors.js` | compute edge render positions per output port |
| `frontend/src/components/Automation/components/nodeEditors/LogicIfEditor.jsx` | surface branch semantics help text |
| `frontend/src/components/Automation/components/nodeEditors/LogicForEditor.jsx` | surface `body` and `after` semantics help text |
| `automation-service/internal/engine/engine_test.go` | add revision 1 to revision 2 migration tests if placed here or in dedicated definition tests |

## Phase 2 files

| File | Change |
| --- | --- |
| `automation-service/internal/infra/db/models.go` | add `SourceKind`, `SourceFormat`, `SourceCode`, and `SourceRevision` to `Workflow` |
| `automation-service/internal/infra/db/repository.go` | add migration for new workflow persistence columns |
| `automation-service/internal/http/workflows.go` | accept `source` payloads, compile code to canonical graph definition, return stored source metadata |
| `automation-service/internal/http/handlers.go` | register conversion routes if route setup lives here |
| `automation-service/internal/http/router.go` | register `POST /api/automations/convert/code-to-graph` and `POST /api/automations/convert/graph-to-code` |
| `automation-service/internal/engine/source_hnac.go` | new parser/compiler from `hnac.v1` to canonical graph definition |
| `automation-service/internal/engine/source_hnac_test.go` | tests for code-to-graph compilation |
| `automation-service/internal/engine/source_export_hnac.go` | new graph-to-code exporter for representable workflows |
| `automation-service/internal/engine/source_export_hnac_test.go` | tests for graph-to-code export and rejection cases |
| `frontend/src/services/automationService.js` | add `convertAutomationCodeToGraph()` and `convertAutomationGraphToCode()` clients |
| `frontend/src/components/Automation/Automation.jsx` | load code-authored workflows, trigger graph conversion when opening in graph mode, preserve source metadata |
| `frontend/src/components/Automation/definition.js` | support editor metadata for code-origin workflows and source maps if returned |
| `frontend/src/components/Automation/components/AutomationTopbar.jsx` | add UI actions for `Refresh graph from code` and `Export graph to code` |
| `frontend/src/components/Automation/components/AutomationCodeEditor.jsx` | new code editor panel or modal for `hnac.v1` source |

## Phase 3 files

| File | Change |
| --- | --- |
| `device-hub/internal/http/handlers.go` | register new `POST /api/hdp/automation/target-profile` route |
| `device-hub/internal/http/automation_target_profile.go` | new handler and response normalization logic |
| `device-hub/internal/http/automation_target_profile_test.go` | new tests for shared inputs and shared state fields |
| `frontend/src/services/deviceHubService.js` | add `describeAutomationTargetProfile()` client |
| `frontend/src/components/Automation/hooks/useAutomationTargetProfile.js` | new hook for device and selector targets |
| `frontend/src/components/Automation/hooks/useAutomationDeviceSelectors.js` | reduce to target pickers or fold into target-profile hook |
| `frontend/src/utils/groupControls.js` | extract reusable state field intersection helpers if needed |
| `frontend/src/utils/deviceFields.js` | ensure common field collection supports automation use cases |

## Phase 4 files

| File | Change |
| --- | --- |
| `frontend/src/components/Automation/components/nodeEditors/ActionSendCommandEditor.jsx` | replace hardcoded command builder with metadata-driven input renderer |
| `frontend/src/components/Automation/definition.js` | compile interactive command UI to canonical `command` and `args` |
| `frontend/src/components/Automation/components/AutomationPropertiesPanel.jsx` | pass target-profile data into action editor |
| `frontend/src/components/Automation/Automation.jsx` | fetch target-profile data based on selected node |
| `frontend/src/components/Automation/components/AutomationCanvas.jsx` | update node subtitle/body summaries for group and selector targets |
| `frontend/src/components/Automation/definition.test.js` | add serialization tests for capability-driven command UI |

## Phase 5 files

| File | Change |
| --- | --- |
| `frontend/src/components/Automation/components/nodeEditors/TriggerEditor.jsx` | replace hardcoded device-state builder with metadata-driven field editor |
| `frontend/src/components/Automation/definition.js` | compile interactive trigger UI to canonical `path`, `value`, and `match_mode` |
| `automation-service/internal/engine/definition.go` | validate `path` and `match_mode`, support legacy `key` migration |
| `automation-service/internal/engine/triggers.go` | match on `path` and selector targets using explicit group semantics |
| `automation-service/internal/engine/engine_test.go` | add trigger path and selector tests |

## Phase 6 files

| File | Change |
| --- | --- |
| `automation-service/internal/engine/execution.go` | add `ExecutionContext`, `vars`, and range loop execution |
| `automation-service/internal/engine/definition.go` | validate `LogicFor.Mode`, `Variable`, `From`, `To`, `Step` |
| `frontend/src/components/Automation/components/nodeEditors/LogicForEditor.jsx` | support `count` and `range` modes plus variable name |
| `frontend/src/components/Automation/components/nodeEditors/LogicIfEditor.jsx` | document and support `vars.*` conditions |
| `frontend/src/components/Automation/definition.js` | serialize richer `logic.for` UI |
| `automation-service/internal/engine/sleep_test.go` | no direct change required, but keep nearby validation style consistent |
| `automation-service/internal/engine/execution_test.go` | new tests for `logic.for` and `logic.if` variable-aware execution |

## Phase 7 files

| File | Change |
| --- | --- |
| `automation-service/internal/engine/triggers.go` | add `match_mode = all` latest-state logic |
| `automation-service/internal/engine/engine.go` | maintain per-device latest state cache if needed |
| `automation-service/internal/engine/targets_test.go` | broaden selector/group tests |
| `automation-service/internal/engine/engine_test.go` | add `any` and `all` group trigger tests |

## Validation And Migration Rules

## Backend validation

- reject unknown `from_port` values
- reject duplicate `then` or `else` ports on a single `logic.if` node
- reject duplicate `body` or `after` ports on a single `logic.for` node
- reject `logic.for.mode = range` when `step == 0`
- reject `action.send_command.wait_for_result = true` for selector targets
- accept legacy `trigger.device_state.key` only during normalization
- reject `source.kind = code` requests when `source.format` is unknown
- reject code-to-graph conversions that cannot compile to canonical graph definitions

## Frontend validation

- show explicit branch labels in the canvas and editor help text
- disable interactive mode when target profile returns no compatible inputs or fields
- preserve JSON mode for unsupported devices, incomplete metadata, or unresolved selectors
- warn when a selector resolves to zero devices
- show source-of-truth state when a workflow is code-authored
- show conversion errors when code cannot be rendered into graph form

## Migration behavior

Load-time migration rules:

- revision 1 edges become revision 2 edges with `from_port`
- revision 1 `logic.for.count` becomes `mode = count`
- revision 1 `trigger.device_state.key` becomes `path = state.<key>`
- missing `schema_revision` becomes `1` on parse and `2` on save
- existing workflow rows without `source_kind` migrate to `source_kind = graph`, `source_format = graph-json`, `source_code = ''`

Save-time behavior:

- always write `schema_revision = 2`
- never write legacy positional branch semantics
- never write legacy `key` if `path` is available
- graph-authored saves update both canonical `definition` and stored graph source metadata
- code-authored saves update canonical `definition` plus `source_code` when the source of truth remains code
- visual edits to code-authored workflows require an explicit source-of-truth decision before save completes

## Testing Matrix

Required automated coverage:

- workflow parse and save migration for revision 1 definitions
- code-to-graph compilation for valid `hnac.v1`
- graph-to-code export for representable graph workflows
- workflow CRUD for `source.kind = code`
- explicit branch execution for `logic.if` then and else
- explicit branch execution for `logic.for` body and after
- selector target resolution for groups
- target-profile shared inputs for homogeneous groups
- target-profile empty shared inputs for heterogeneous groups
- capability-driven command serialization
- capability-driven trigger serialization
- range loop execution and `vars.i` usage in `logic.if`

## Rollout Notes

- Phase 1 through Phase 3 can ship before `match_mode = all` is fully implemented.
- Code-first storage can ship before the full code editor UX as long as conversion APIs and graph-open behavior are available.
- The first user-visible win should be branch-stable graphs plus capability-driven command editing for device and group targets.
- Trigger field upgrades should ship after target-profile API exists so command and trigger editors share the same metadata normalization.

## Final Acceptance Criteria

The implementation is complete when all of the following are true:

- groups are valid and usable targets for device commands and device state triggers
- interactive device command options are derived from HDP metadata inputs, not hardcoded light presets
- interactive trigger field options are derived from HDP metadata state fields, not only current state keys
- automations can be stored as either graph JSON or `hnac.v1` code while still producing the same canonical runtime definition
- code-authored automations can be opened in the graph editor through deterministic conversion
- `logic.if` exposes stable explicit `then` and `else` ports
- `logic.for` exposes stable explicit `body` and `after` ports and supports loop variables
- existing workflows continue to load through migration without manual repair