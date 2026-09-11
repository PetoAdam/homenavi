# Dashboard Column-Mode Layout Refactor Plan

## Goal

Refactor the Home dashboard so that layout behavior is consistent across viewport sizes, layout persistence is keyed by column count rather than screen breakpoint name, and layout editing tools are safe, predictable, and reversible.

This plan is intentionally implementation-focused. It covers:

- monotonic column-mode behavior across viewport sizes
- column-count-based layout storage
- SQL-driven one-time data migration
- layout generation and dense reflow rules
- import / auto-arrange / regenerate tools with preview and undo
- first-time edit tutorial flow
- widget-priority-aware placement rules
- unit, integration, and screenshot-driven validation

This document does not implement the change. It defines the intended design and rollout plan.

## Problem Summary

The current dashboard behavior mixes several concerns that should be independent:

- viewport breakpoints
- sidebar mode and width
- effective content width
- stored dashboard layout identity

That causes several UX and architecture problems:

- a smaller screen can end up with more columns than a slightly larger screen because the sidebar mode changes
- saved layouts are keyed by breakpoint names rather than true layout modes
- `md`, `lg`, and `xl` can all be separate layouts even when they represent the same practical density target
- 3-column behavior is hard to manage because it is neither a clean factor-of-2 transform nor a first-class storage concept
- adding widgets across layouts relies on weak placement heuristics and can create sparse or awkward results
- future layout tools such as import or auto-arrange would be risky without preview and undo

## Target Product Model

### Core principles

1. Column count is a first-class layout mode.
2. A narrower viewport must never increase the number of columns.
3. Sidebar mode must not redefine layout identity.
4. Widget instances and widget settings are shared across the dashboard.
5. Geometry is stored per column mode only.
6. Derived layouts may be generated from nearby stored layouts, but generation must be deterministic and reversible.

### Column modes

The dashboard should use semantic layout modes instead of breakpoint-name layouts:

- `4`
- `3`
- `2`
- `1`

These are the only persisted layout identities.

### Viewport-to-column mapping

The exact thresholds should be tuned during implementation, but the behavior must be monotonic.

Recommended initial target:

- `4 columns`: large desktops only
- `3 columns`: most laptops and medium desktop windows
- `2 columns`: smaller tablets / narrow laptop windows
- `1 column`: phones and very narrow windows

Recommended initial threshold candidates:

- `4 columns`: `>= 1600px`
- `3 columns`: `1200px - 1599px`
- `2 columns`: `760px - 1199px`
- `1 column`: `< 760px`

These numbers are not final acceptance thresholds. They are starting points for the screenshot validation loop.

### Sidebar mode

Sidebar mode remains a separate shell concern.

Recommended initial target:

- permanent sidebar: `>= 1200px`
- overlay sidebar: `< 1200px`

Important rule:

The dashboard column mode is derived from viewport width bands, not from measured post-sidebar content width. That prevents the current inconsistency where a smaller screen can jump back to a denser layout after the sidebar switches to overlay mode.

### Sidebar width model

The permanent sidebar should not remain a single fixed width across all larger screens.

Recommended shell rule:

- permanent sidebar width interpolates between a minimum and maximum width
- the minimum width must preserve comfortable readability, padding, icon spacing, and label clarity
- the maximum width must not exceed the current fixed desktop width budget, because wider values only create visual emptiness on large screens

Recommended product constraints:

- keep the current fixed desktop width as the permanent maximum cap
- do not shrink the minimum so far that labels, section headers, or menu-item spacing start feeling compressed
- treat the sidebar as a readability surface, not only a space-recovery lever

The exact interpolation curve can stay linear unless screenshots show that a non-linear curve is clearly better.

## Storage Model

### Persisted document shape

Move from breakpoint-name storage to column-count storage.

Current conceptual shape:

```json
{
  "items": [...],
  "layouts": {
    "lg": [...],
    "md": [...],
    "sm": [...]
  }
}
```

Target conceptual shape:

```json
{
  "items": [
    {
      "instance_id": "widget-1",
      "widget_type": "homenavi.weather",
      "enabled": true,
      "settings": {}
    }
  ],
  "layouts_by_cols": {
    "4": [{ "i": "widget-1", "x": 0, "y": 0, "w": 2, "h": 6 }],
    "3": [{ "i": "widget-1", "x": 0, "y": 0, "w": 2, "h": 6 }],
    "2": [{ "i": "widget-1", "x": 0, "y": 0, "w": 2, "h": 6 }],
    "1": [{ "i": "widget-1", "x": 0, "y": 0, "w": 1, "h": 6 }]
  }
}
```

### Shared vs per-mode data

Shared across all column modes:

- widget instance identity
- widget type
- widget settings
- enabled / disabled state

Stored per column mode only:

- `x`
- `y`
- `w`
- `h`
- any future geometry-only metadata

### Why not fully separate dashboards per size?

That would increase flexibility, but it creates unnecessary product complexity:

- users would have to maintain multiple dashboards conceptually, not one
- changes to widgets and settings would drift between modes
- testing burden would rise sharply
- layout tools would become much harder to reason about

The better enterprise compromise is:

- one dashboard
- shared widget instances and settings
- separate geometry per column mode

## Data Compatibility and Migration Strategy

### Migration direction

We should migrate existing documents to the new shape once, rather than carrying long-lived frontend compatibility logic.

Because this work is still pre-`v1.0`, the migration should be executed as a proper SQL data migration against persisted dashboard rows, not as permanent compatibility code in the frontend.

This means:

- do not keep a long-term app-layer migration shim for the old breakpoint-keyed model
- write explicit SQL commands to reshape stored dashboard docs into the new `layouts_by_cols` form
- validate those SQL transformations against real representative dashboard rows before rollout
- keep the migration logic documented and reproducible for local, staging, and production-like environments

Code changes are still required for the new storage contract itself, but the one-time conversion of existing user data should be done in SQL.

### Migration rules

1. Read existing `layouts` keyed by breakpoint names.
2. Map them into `layouts_by_cols` using the current intended semantic mapping.
3. Collapse duplicate desktop-class layouts where the breakpoint layouts represent the same intended mode.
4. Preserve user-authored geometry whenever possible.
5. If multiple source layouts map to the same target column mode, prefer the densest valid layout unless a product rule says otherwise.

### Migration execution policy

Recommended execution flow:

1. export or snapshot the affected dashboard rows
2. run SQL queries that transform the JSON documents in place
3. verify the resulting rows with targeted SQL inspection queries
4. clear any dashboard read cache after migration
5. validate the migrated data in the running UI

The migration should be expressed as explicit SQL scripts or runbook commands, not hidden inside frontend runtime code.

### Initial mapping policy

Recommended migration policy:

- `4 cols`: prefer `xl`, then `lg`, then `md`
- `3 cols`: prefer `md`, then `lg`, then generated-from-4, then generated-from-2`
- `2 cols`: prefer `sm`, then generated-from-3`, then generated-from-4`, then `xxs`
- `1 col`: prefer `xxs`, then `xs`, then generated-from-2`

This should be documented in the migration implementation to avoid future ambiguity.

### Default dashboard seed

The backend default dashboard must also move to the new column-mode storage model. The default document should define all four modes explicitly.

## Rendering Model

### Active layout resolution

At render time:

1. Determine the active column mode from viewport width.
2. Load that exact layout if stored.
3. If missing, generate a derived layout from the nearest stored mode.
4. Render the layout without persisting it immediately.
5. Persist only when the user edits, confirms an import / auto-arrange action, or explicitly saves a generated layout.

### Fallback generation order

Recommended nearest-mode fallback order:

- missing `4`: `3`, then `2`, then `1`
- missing `3`: `4`, then `2`, then `1`
- missing `2`: `3`, then `1`, then `4`
- missing `1`: `2`, then `3`, then `4`

This should be deterministic and test-covered.

## Dense Reflow and Auto-Arrangement

### Objective

When generating or restructuring a layout, the system should:

- maximize density
- minimize holes
- preserve the user’s visual priority ordering as much as possible
- avoid making important widgets disappear into worse positions
- remain deterministic

### Priority model

Priority should be derived from the source layout’s reading order.

Recommended weight rule:

- widgets farther up are more important
- widgets farther left are more important when on the same row
- earlier items in stable reading order break ties

Conceptually:

- primary sort: `y`
- secondary sort: `x`
- tertiary sort: stable source order

This should become the base widget priority for generated layouts.

### Reflow algorithm requirements

The conversion algorithm should:

1. Build a stable ordered list from the source layout.
2. Clamp widget widths to the target column count.
3. Apply widget-type span preferences for the target mode.
4. Place widgets using a dense packing strategy.
5. Run a hole-filling compaction pass.
6. Preserve relative importance whenever density tradeoffs are close.

### Recommended algorithm shape

Recommended implementation shape:

- stable reading-order extraction
- widget span normalization by target mode
- skyline or shortest-column placement for initial packing
- upward compaction pass
- optional local swap pass for simple gap elimination

This is preferable to naive scale-down or naive prepend-and-compact logic.

### Widget span policy

Each widget type should be able to express preferred spans by column mode.

Examples:

- weather: prefers more width in `4` and `3`
- map: prefers wide spans when available
- manual trigger: often acceptable at `1`
- device summary cards: often acceptable at `1`, but some variants may prefer `2`

The exact policy should remain in a widget registry layer, not be scattered through the layout reducer.

## Edit-Mode Tools

### Required tools

Add the following layout-management tools in dashboard edit mode:

1. `Import layout from...`
2. `Auto-arrange current layout`
3. `Regenerate selected layouts`

### Import layout from...

Purpose:

- copy the geometry from another column mode into the current one
- optionally reflow it to fit the target mode before previewing

Recommended UX:

- select source mode
- show preview diff
- allow confirm or cancel

### Auto-arrange current layout

Purpose:

- re-pack the current mode for better density without changing widget settings

Recommended UX:

- preview generated result
- confirm or cancel
- if confirmed, create an undo checkpoint

### Regenerate selected layouts

Purpose:

- rebuild one or more target layouts from a chosen source mode using the dense reflow algorithm

Recommended UX:

- choose source mode
- choose target modes
- preview one mode at a time or toggle between them
- confirm or cancel

## Preview and Undo Requirements

These tools must not be destructive by default.

### Preview model

Each layout transformation should first produce a preview state separate from the persisted doc.

Recommended approach:

- preview lives only in local edit-mode state
- preview can be compared against current saved geometry
- preview is applied to the grid only after the user chooses to inspect it
- user can discard preview without mutating the stored layout

### Undo model

At minimum, support one-step undo for any confirmed layout transformation.

Recommended approach:

- create a snapshot before import / auto-arrange / regenerate confirmation
- keep an in-session undo stack per dashboard edit session
- expose `Undo last layout change`

Implementation note:

The frontend already has a good precedent for this in the Map and Automation editors. Both use the shared `useEditorHistory` hook pattern, including snapshot comparison, undo / redo stacks, and batched history updates where appropriate. The dashboard should reuse this architecture rather than invent a second unrelated history system.

Relevant existing precedents:

- Map editor persistence and history wrapper: `frontend/src/components/Map/services/mapController/usePersistedLayout.js`
- Automation editor history wiring: `frontend/src/components/Automation/Automation.jsx`
- Shared hook: `frontend/src/hooks/useEditorHistory.js`

Optional later enhancement:

- multi-step undo/redo within edit mode only

### Redo model

Because the shared history pattern already supports redo, the dashboard plan should treat redo as part of the first implementation, not as a much-later luxury feature, if the UX remains clean.

## Frontend Architecture Plan

### State separation

Separate these concerns explicitly:

- shell layout state: sidebar mode, viewport width band
- active dashboard column mode
- persisted dashboard doc
- transient edit session state
- transient preview state
- undo stack

### Suggested frontend modules

- `dashboardColumnMode.js`
  - viewport to column-mode mapping
- `layoutStorage.js`
  - doc parsing / serialization for `layouts_by_cols`
- `layoutGeneration.js`
  - nearest-layout generation, import, regenerate, reflow
- `layoutPriority.js`
  - stable order and weight helpers
- `layoutHistory.js`
  - preview checkpoints and undo stack helpers
- `dashboardOnboarding.js`
  - first-time edit tutorial state and dismissal helpers
- widget registry extensions
  - per-widget preferred spans / mode hints

### Current reducer implications

The current dashboard UI reducer likely needs to grow beyond modal + edit flags to include:

- active column mode
- preview mode state
- pending transformation metadata
- undo checkpoints

The implementation should avoid turning the reducer into a dumping ground. Keep transformation logic in dedicated pure helpers.

### First-time edit tutorial

Add a small first-time tutorial for dashboard editing.

Requirements:

- show only for users entering dashboard edit mode for the first time, or until dismissed
- include a skip option
- do not block the user from continuing once skipped
- keep it short and contextual, not a long walkthrough

Recommended tutorial steps:

1. explain that widgets can be dragged and resized by width
2. show where settings and remove actions live
3. explain import / auto-arrange / regenerate tools
4. explain preview and undo / redo safety
5. point to the Done action for saving and exiting edit mode

Persistence:

- store in db properly

## Backend Plan

### Dashboard document contract

Update the backend dashboard document contract to support `layouts_by_cols` as the primary persisted geometry model.

### Backend responsibilities

The backend should:

- accept and return the new document shape
- seed new dashboards with all column modes
- provide migration support for existing persisted docs

The backend should not become responsible for rich layout editing logic unless there is a later requirement for server-side normalization. The main transformation engine should stay client-side for now.

### Optional backend validation

Optional but recommended validation rules:

- only known column-mode keys are accepted
- no item width may exceed its column-mode count
- no layout items may refer to missing widget instances

## Testing Strategy

This work must be validated at three layers.

### 1. Unit tests

Required test areas:

- viewport width to column-mode mapping
- nearest-layout fallback selection
- breakpoint-to-column migration mapping
- dense reflow placement determinism
- priority preservation rules
- hole-filling / compaction behavior
- widget add propagation across all column modes
- import preview generation
- auto-arrange preview generation
- regenerate preview generation
- undo checkpoint creation and restoration
- redo restoration after undo
- first-time tutorial visibility / dismissal behavior

Key test scenarios:

- import 4 -> 3
- import 3 -> 2
- generate 3 from 4 when no 3 exists
- generate 2 from 3 when no 2 exists
- add a wide widget and clamp / preserve sensibly
- preserve top-left important widgets through conversions
- cancel preview leaves persisted doc unchanged
- confirm preview persists only the accepted modes

### 2. Local integration tests

Use the local Compose deployment path:

```bash
dc up -d --build
```

Required integration validation:

- dashboard loads with migrated and fresh docs
- SQL migration runbook works against representative local data
- edit mode works across all 4 column modes
- add widget propagates across all stored layouts
- import / auto-arrange / regenerate preview flows behave correctly
- undo restores the pre-transformation layout
- redo reapplies the reverted transformation correctly
- first-time tutorial can be skipped and does not reappear incorrectly
- save / reload round-trip preserves the intended geometry

### 3. Screenshot-driven browser validation

This is required for final tuning because layout quality is visual, not purely logical.

Required validation loop:

1. seed dashboards with several representative widget mixes
2. render at multiple viewport sizes
3. capture screenshots
4. review density, truncation, whitespace, and widget span quality
5. tune thresholds, span rules, and compacting rules
6. repeat until acceptable

Required viewport set for screenshot review:

- large desktop 4-col
- medium desktop 4-col
- laptop 3-col
- narrow laptop / tablet 2-col
- phone 1-col

Representative seeded dashboards:

- default 4-widget starter dashboard
- dense mixed dashboard with 12-16 widgets
- device-heavy dashboard
- trigger-heavy dashboard
- map + weather + device mix
- long-label dashboard with difficult truncation cases

Review criteria:

- density
- visual balance
- truncation severity
- holes and wasted grid space
- preservation of important top-left widgets
- consistency between generated and manually edited layouts

## Suggested Seed / Fixture Strategy

Build deterministic dashboard fixtures for both unit and visual testing.

Recommended fixture sets:

- `starter-dashboard`
- `dense-admin-dashboard`
- `device-heavy-dashboard`
- `mixed-media-dashboard`
- `stress-long-labels-dashboard`

Each fixture should define:

- shared widget instances
- at least one canonical source layout
- expected generated layouts for selected target modes

## Rollout Plan

### Phase 1. Finalize product rules

- lock viewport-to-column thresholds
- lock sidebar vs column-mode independence
- lock sidebar min/max width bounds and interpolation policy
- lock doc shape and fallback semantics
- lock preview / undo UX rules
- lock first-time tutorial behavior

Deliverable:

- approved design contract

### Phase 2. Introduce column-mode helpers and tests

- add pure helpers for mode selection, fallback selection, ordering, and reflow
- add tests first for these helpers

Deliverable:

- deterministic transformation core with coverage

### Phase 3. Backend doc contract update

- add support for `layouts_by_cols`
- update default dashboard seed
- prepare SQL migration tooling for existing docs

Deliverable:

- backend accepts the new document shape

### Phase 4. Frontend storage and render refactor

- move active render logic from breakpoint-keyed layouts to column-mode layouts
- keep rendering stable during migration window

Deliverable:

- dashboard renders from column-count layouts

### Phase 5. Edit tools with preview and undo

- implement import
- implement auto-arrange
- implement regenerate selected layouts
- implement preview and undo / redo stack
- implement first-time tutorial with skip

Deliverable:

- safe transformation workflows

### Phase 6. Compose validation and screenshot loop

- run the local environment
- run and verify SQL migration against representative local dashboard rows
- seed representative dashboards
- capture screenshots at multiple widths
- tune thresholds and reflow rules

Deliverable:

- visually validated behavior

### Phase 7. Cleanup and documentation

- update the dashboard functional spec
- document the new storage model
- document transformation semantics

Deliverable:

- current documentation matches shipped behavior

## Acceptance Criteria

The work is complete when all of the following are true:

- narrower viewports never increase dashboard columns
- dashboard layouts are stored by column count, not breakpoint name
- migration from the old breakpoint-keyed model is executed through explicit SQL, not through long-lived compatibility code
- the active layout for a mode is deterministic
- missing layouts can be generated from the nearest mode predictably
- add widget propagates to every layout mode without creating obviously sparse results
- import / auto-arrange / regenerate all provide preview and undo / redo
- confirmed layout transforms preserve priority ordering sensibly
- first-time edit tutorial is available, skippable, and non-intrusive
- local Compose validation succeeds
- screenshot review across target sizes is judged visually acceptable

## Recommended Product Defaults

If no contrary requirement appears during implementation, use these defaults:

- one shared dashboard with shared widget settings
- separate stored geometry for `4`, `3`, `2`, `1`
- missing modes generated from nearest mode
- transformations previewed before persistence
- explicit import tool rather than silent cross-mode overwrites

This is the best balance between enterprise-grade predictability, user control, and maintainable architecture.