# Dashboard UI – Functional Specification

This document describes the intended end-user behavior of the **customizable Home dashboard** UI (widgets, editing, persistence) as implemented in the current frontend.

## 1) Scope & goals

The dashboard provides:
- A **widget-based homepage** with a responsive, column-based layout.
- A dedicated **Edit mode** for rearranging/removing widgets and changing widget settings.
- **Persistent layouts** stored per-user via the backend.

Non-goals:
- This spec does not define marketplace/integration widgets beyond the core catalog.
- This spec does not define backend storage implementation details beyond what affects UX.

## 2) Roles and access

- Only users with role **resident** or **admin** can use the dashboard.
- When the user lacks permission, the UI shows an unauthorized view (no dashboard content).

## 3) Layout model (columns, snapping, breakpoints)

### 3.1 Breakpoints and columns

The dashboard layout is **column-based** and responsive:
- `lg`: 4 columns
- `md`: 3 columns
- `sm`: 2 columns
- `xs`: 1 column
- `xxs`: 1 column

Widgets always **snap to these columns** (no free-form X positioning).

### 3.2 Widget sizing

- Default widget width is **1 column**.
- In Edit mode, a widget’s width can be changed to span multiple columns via **horizontal resize handles**.
- Widget height is **automatic** (content-driven). Users do not resize height directly. Like the height of a device widget depends on how many fields it has. The height of a map widget depends on the width of the widget to preserve aspect ratio.

### 3.3 Non-overlap guarantee

After load (and after edit-mode changes), the UI compacts the layout vertically so that:
- Widgets are placed top-to-bottom in their columns.
- The persisted layout is repaired if necessary to ensure **no overlaps**.

## 4) Dashboard persistence

### 4.1 Endpoint

The dashboard is loaded and saved using:
- `GET /api/dashboard/me`
- `PUT /api/dashboard/me`

### 4.2 Stored document shape (UX-relevant)

The persisted “doc” contains:
- `items`: widget instances with `instance_id`, `widget_type`, `enabled`, `settings`.
- `layouts`: a map of breakpoint name → array of layout items `{ i, x, y, w, h }`. Please check the backend code for more info. Feel free to change it if necessary.

### 4.3 Concurrency and saving behavior

- The frontend saves changes through a queued writer so rapid drag/resize changes are not lost.
- If a save fails due to a version conflict, the UI reloads the latest dashboard from the backend.

### 4.4 When changes are saved

- Dragging a widget, resizing its width, adding/removing widgets, or updating widget settings results in the dashboard doc being saved.
- Saving is **debounced** during rapid interactions and flushed when the user presses **Done**.

## 5) Interaction design

## 5.1 Modes

### View mode (default)
- Widgets are **interactive** (e.g., device controls, weather scrolling).
- Layout changes are disabled.

### Edit mode
- Widgets can be rearranged and resized (width only - only be able to set how many columns the width is).
- Widgets’ internal UI is visually de-emphasized and interaction-disabled (to make dragging easier).
- An edit overlay appears on each widget (not above it - to preserve the sizes and dashboard layout between view and edit mode), and a bottom **trash drop zone** becomes available.

## 5.2 Floating action buttons (FAB)

Bottom-right FABs control mode:
- **Edit**: enters Edit mode.
- In Edit mode:
  - **Add**: opens Add Widget modal.
  - **Done**: exits Edit mode and flushes pending saves.

## 5.3 Per-widget edit overlay

When in Edit mode, each widget tile shows:
- **Drag handle** (grip icon): the only place where dragging starts.
- **Settings** (gear icon): opens that widget’s settings.
- **Remove** (trash icon): removes the widget immediately.

## 5.4 Dragging and snapping

- Dragging is only allowed in Edit mode.
- Dragging uses a fixed grid and snaps to columns.
- Widgets never overlap after drop; the layout compacts vertically.

## 5.5 Removing widgets via trash drop zone

In Edit mode:
- A trash zone appears near the bottom.
- Dragging a widget over the trash zone highlights the zone.
- Releasing the widget over the zone removes it.

## 5.6 Adding widgets

In Edit mode, clicking **Add** opens the Add Widget modal:
- Contains a search field.
- Shows the widget catalog as cards.
- Clicking **Add** on a card creates a new instance of that widget.

## 5.7 Widget settings

Widget settings are opened from the gear icon in Edit mode.

- Widgets with custom settings UI (Device, Weather) show their own settings modal.
- Other widgets show a generic settings modal for common fields (currently: Title).

## 6) Core widgets

Widget types come from the catalog (`GET /api/widgets/catalog`). The core widgets used by the dashboard are:

### 6.1 Device widget (`homenavi.device`)

Purpose:
- Show a single device’s key controls and key state fields.

View behavior:
- If a device is selected, the widget renders a device card-style view.
- Controls are interactive.
- If the device has a detail page, an **Open** action navigates to the device page.

Edit behavior:
- The device card is displayed in a non-interactive preview state.
- Settings are editable via the settings modal.

Settings:
- **Title** (optional): override the widget title; default uses device name.
- **Fields layout**:
  - `cards`: fields displayed as metric cards.
  - `list`: fields displayed as a simple list layout.
- **Device selection**:
  - Select from discovered devices, or paste an ID.
- **Controls list**:
  - Choose which controls to show.
  - Reorder controls.
  - Remove controls.
  - If the user explicitly sets an empty list, the widget respects it (shows no controls).
- **Fields list**:
  - Choose which fields (state keys) to show.
  - Reorder fields.
  - Remove fields.
  - If the user explicitly sets an empty list, the widget respects it (shows no fields).

Color control behavior:
- If a device exposes a color input, it uses the same **hex color picker UI** as the Devices page.

Empty state:
- If no device is selected, the widget shows “No device selected.”

Also, many parts from the device list device cards are to be reused and made into common react components for simple reusability.

### 6.2 Weather widget (`homenavi.weather`)

Purpose:
- Show current weather + short forecast.

View behavior:
- Displays current temperature and conditions.
- Forecast carousel supports scrolling and left/right arrow buttons.

Edit behavior:
- Widget is shown as a preview (interaction disabled).

Settings:
- **Title** (optional).
- **Location** (city string): used for the weather query.

Data source:
- `GET /api/widgets/weather`.

Note: you can keep the current look from the placholder widget, it looks stunning.

### 6.3 Map widget (`homenavi.map`)

Purpose:
- Show the Home map (rooms and devices).

Sizing:
- Map defaults to preserving aspect ratio - height depends on widget width.

Edit behavior:
- Map interactions are disabled to make drag/resize easier.

### 6.4 Automation / Workflow widget (`homenavi.automation.manual_trigger`)

Purpose:
- Run a workflow that includes a **manual trigger**.

View behavior:
- Shows Run + Open buttons.
- Running a workflow displays a progress bar and step status.

Edit behavior:
- Pick a workflow from a list (filtered to workflows that contain a manual trigger).
- Optionally paste a workflow ID.

Data source:
- Workflows list: `GET /api/automation/workflows`.
- Run: `POST /api/automation/workflows/{id}/run` (triggered via UI).
- Run status: polled via `GET /api/automation/runs/{runId}`.

## 7) UX expectations (acceptance criteria)

A dashboard build is considered correct if:
- Widgets can only be dragged/resized in Edit mode.
- Dragging snaps to columns and cannot produce overlaps after drop.
- Reloading the page restores the same layout without overlaps.
- Users can resize widget **width** (columns) in Edit mode.
- Widget height is automatic and adjusts to content.
- Device widget color input uses the same picker UX as the Devices page.

## 8) Troubleshooting notes

- If behavior appears unchanged after deployment, verify the browser is not serving a stale bundle (a service worker may cache the app). Unregister the service worker and clear site data if necessary.

## 9)

- Please also make sure to follow doc/dashboard_widgets_integrations_marketplace_roadmap.md so that there is a default handling for widgets and 3rd parties can use thos to create their 3rd party integrations and make it simple for them to work with homenavi. Please also make the styling of the widgets consistent.
