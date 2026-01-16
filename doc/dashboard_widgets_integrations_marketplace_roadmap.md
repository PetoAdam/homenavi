# Dashboard Widgets + Integrations + Marketplace — Architecture + Detailed Roadmap

This document defines a detailed, end-to-end plan to add:

- A **customizable Home dashboard** (per-user, with a default template).
- A **Widget Manager / Dashboard service** for persistence.
- A secure **widget runtime** in the Frontend that gracefully degrades on auth/role failures.
- A plugin-style **Integrations system** that can provide:
  - Dashboard widgets
  - Automation triggers/actions/conditions
  - A dedicated “Integration” UI surface (sidebar tab)
- A **centralized marketplace + verification** system (hosted outside deployments, e.g. `https://store.homenavi.org`).
- A future **admin control plane** for managing the Homenavi ecosystem (enable/disable services, restart/update, keep configs).

This is designed to fit the existing Homenavi patterns:

- **API Gateway** routing and centralized auth.
- **JWT + roles** (resident/admin) with many endpoints returning **401/403**.
- Existing UI patterns like a “no access” widget:
  - [frontend/src/components/common/NoPermissionWidget/NoPermissionWidget.jsx](../frontend/src/components/common/NoPermissionWidget/NoPermissionWidget.jsx)
- Existing docs that already anticipate per-user widgets (currently in ERS proposal):
  - [doc/ers_service_proposal_and_roadmap.md](ers_service_proposal_and_roadmap.md)

## Scope boundaries (explicit)

- **Single-home only**.
  - One Homenavi deployment == one home.
  - Multi-home is handled by multiple deployments (mobile app “home selector” is out of scope here).
- The dashboard is **per user** with a **default dashboard template** used to bootstrap new users.
- Security is **first-class**: third-party widgets must not be able to steal tokens or silently exfiltrate data.
- Marketplace/verification is a **central service** (not deployed inside individual homes).

---

## Table of Contents

1. [Goals](#1-goals)
2. [Non-Goals (v1)](#2-non-goals-v1)
3. [Current State (repo anchors)](#3-current-state-repo-anchors)
4. [Target Architecture (services + responsibilities)](#4-target-architecture-services--responsibilities)
5. [Core Concepts + Data Model](#5-core-concepts--data-model)
6. [Security Model (verified/unverified + token safety)](#6-security-model-verifiedunverified--token-safety)
7. [Widget Runtime (frontend architecture)](#7-widget-runtime-frontend-architecture)
8. [Backend APIs (dashboard + catalog + integrations)](#8-backend-apis-dashboard--catalog--integrations)
9. [Integrations (discovery + manifests)](#9-integrations-discovery--manifests)
10. [Automation Extensions from Integrations](#10-automation-extensions-from-integrations)
11. [Marketplace (store.homenavi.org) + Verification Pipeline](#11-marketplace-storehomenaviorg--verification-pipeline)
12. [Admin Panel Roadmap (integrations + ecosystem control)](#12-admin-panel-roadmap-integrations--ecosystem-control)
13. [Phased Delivery Plan (milestones + acceptance criteria)](#13-phased-delivery-plan-milestones--acceptance-criteria)
14. [Testing Strategy](#14-testing-strategy)
15. [Operational Considerations (deploy, updates, backups)](#15-operational-considerations-deploy-updates-backups)
16. [Third-Party Developer Documentation Plan](#16-third-party-developer-documentation-plan)
17. [Open Questions / Decisions](#17-open-questions--decisions)

---

## 1. Goals

### 1.1 Home dashboard customization

- Users can **edit their home dashboard**:
  - Add widgets
  - Remove widgets
  - Reorder widgets via drag-and-drop
  - Resize widgets (optional in v1; see mobile constraints)
- Dashboard configuration persists and is restored on reload.

### 1.2 Default dashboard template

- A default dashboard exists “to start with” (initially controlled by admin).
- When a new user first loads Home:
  - Their dashboard is created by cloning the default.

### 1.3 Permission-aware widget fallbacks

- Widget data calls can return 401/403.
- The dashboard must not fail “as a page”; instead:
  - the widget tile must show a standard “No access” fallback (similar to map widget behavior today).

### 1.4 Extensibility: integrations provide widgets and automation

- Third-party integrations can supply:
  - Dashboard widgets
  - Automation steps (actions/triggers/conditions)
- Integrations have a dedicated UI surface (sidebar tab) for:
  - configuration
  - connection status (OAuth)
  - logs/diagnostics

### 1.5 Central marketplace + verified tag

- `store.homenavi.org` hosts:
  - integration listings
  - release metadata
  - signatures
  - verified status
- Homenavi deployments can install integrations from the centralized store.
- Widgets/integrations can be “Verified” (reviewed) or “Unverified” (user-installed / community).

---

## 2. Non-Goals (v1)

These are explicitly out of scope for the first shipping slice.

- Multi-home UX within one deployment.
- A full Kubernetes operator / multi-node orchestration.
- Full offline marketplace mirroring.
- Arbitrary code execution administration (“run shell commands”) — **never**.

---

## 3. Current State (repo anchors)

### 3.1 Current Home page

Home is a fixed set of cards today:

- [frontend/src/components/Home/Home.jsx](../frontend/src/components/Home/Home.jsx)

### 3.2 Existing “No access” widget UI

- [frontend/src/components/common/NoPermissionWidget/NoPermissionWidget.jsx](../frontend/src/components/common/NoPermissionWidget/NoPermissionWidget.jsx)

This should become the standard fallback for dashboard widget tiles.

### 3.3 Existing role patterns in UI

Several pages already gate by role and show UnauthorizedView.

For dashboard tiles, we want **tile-level** fallback rather than page-level gating.

---

## 4. Target Architecture (services + responsibilities)

### 4.1 Services overview

Add these services over time (phased):

1) **dashboard-service** (Widget Manager / Dashboard persistence)
- Stores per-user dashboards + default template.
- Stores widget instances + user settings.
- Emits a widget catalog (first-party + integration-provided).

2) **integration-registry-service** (local deployment registry)
- Tracks installed integrations in this home deployment.
- Pulls and validates integration manifests.
- Exposes “capabilities catalog” (widgets + automation steps + integration UI tabs).
- Note: integrations should follow a framework / predefined directory structure

3) **widget-proxy-service** 
- Enforces safe, scoped data access for sandboxed widgets.
- Issues short-lived widget tokens and proxies allowed operations.

4) **admin-control-plane-service** (later; high risk)
- Admin-only operations:
  - enable/disable services
  - restart/update services
  - manage configs and secrets

### 4.2 Centralized services (not deployed per home)

1) **Marketplace / Store** (`store.homenavi.org`)
- A public service used by all deployments.
- Provides:
  - integration metadata
  - versions
  - signatures
  - verification status
  - optionally hosted bundles

2) **Verification pipeline infrastructure**
- CI that builds, analyzes, and signs approved integrations.
- Note: integrations should follow a framework / predefined directory structure. This only works for those.

### 4.3 Data ownership boundaries

- dashboard-service owns:
  - dashboard layouts and widget settings
- existing services remain source of truth for domain data:
  - device-hub/HDP for realtime state
  - automation-service for runs
  - ERS for rooms/tags/logical devices (when introduced)

---

## 5. Core Concepts + Data Model

### 5.1 Dashboard

A dashboard is a list of widget instances plus layout metadata.

**Dashboard** fields:
- `dashboard_id` (UUID)
- `scope`: `user` | `default`
- `owner_user_id` (nullable; null for `default`)
- `title`
- `layout_engine`: `masonry-v1` (recommended for v1)
- `layout_version` (int; optimistic concurrency)
- `items[]`: widget instances
- timestamps

### 5.2 Widget instance
All widgets must extend a predefined BaseWidget component, which handles default styling and errors such as 401 or 403 issues.

Widget instance fields:
- `instance_id` (UUID)
- `widget_type` (string, stable identifier)
- `title_override` (optional)
- `layout`:
  - for masonry: `order` + optional `size_hint`
  - for grid: `x,y,w,h` (future)
- `settings` (JSON)
- `enabled` (bool)

### 5.3 Widget type

A widget type is a definition, not a user instance.

Widget type metadata:
- `id` (e.g. `homenavi.weather`, `integration.spotify.player`)
- `display_name`
- `description`
- `icon`
- `default_size_hint` (e.g. `sm|md|lg`)
- `settings_schema` (JSON schema-ish)
- `data_scopes` (if sandboxed)
- `verified` (boolean, plus `verified_by`)

### 5.4 Default dashboard behavior (copy-on-first-view)

Recommended for v1:

- `GET /api/dashboard/me`:
  - if user dashboard exists: return it
  - else: clone the `default` dashboard into a new user dashboard and return it

This avoids complicated “overlay” logic while still meeting the requirement.

---

## 6. Security Model (verified/unverified + token safety)

Your key requirement: prevent third-party widgets from doing bad things like stealing access tokens.

### 6.1 Threat model (what we defend against)

- A widget author attempts to:
  - read the user’s JWT (from localStorage / memory)
  - call arbitrary internal APIs with that JWT
  - exfiltrate data to arbitrary servers
  - render deceptive UI to phish credentials
  - escape sandbox to interact with parent window

### 6.2 Core principles

1) **Never share the primary user JWT with third-party widget code**.
2) Treat unverified widgets as hostile.
3) Minimize widget capabilities with explicit scopes.
4) Ensure the host (frontend) controls networking access.

### 6.3 Verified vs Unverified

- **Verified**:
  - published through the central store
  - reviewed/approved
  - signed by store keys
  - can show a “Verified” badge
- **Unverified**:
  - user-installed from URL / local bundle
  - unsigned or only developer-signed
  - clearly marked as unverified
  - more restrictive by default

### 6.4 Sandboxed widgets (recommended architecture)

Render third-party widgets inside an iframe with:

- `sandbox` attribute:
  - allow-scripts
  - allow-forms (optional)
  - DO NOT allow-same-origin (prevents cookie/localStorage access)
  - DO NOT allow-top-navigation
- Strong Content Security Policy on widget content:
  - restrict `connect-src` to `self` (or none) to prevent exfiltration
  - restrict `img-src` etc.

Important: to enforce CSP, widget content must be served with headers. This strongly favors serving widgets from integration containers or a controlled proxy, not arbitrary remote URLs.

### 6.5 Host-mediated data access via widget-proxy

Widgets should request data/actions via a host API, not direct network.

Flow:

1) Host (frontend) obtains a **Widget Session Token** (WST) from widget-proxy:
   - short-lived (e.g. 5 minutes)
  - scoped to `widget_instance_id`, `user_id`, `integration_id`
  - carries the **same role claims as the user** who requested it (e.g. `resident` / `admin`) for simplicity and consistent authorization behavior
  - scoped to explicit permissions (capability scopes) in addition to role, e.g. `ers.devices.read`, `automation.run.create`
2) Widget iframe communicates with host via `postMessage`.
3) Host calls widget-proxy with WST.
4) widget-proxy validates role + scopes + rate limits.
5) widget-proxy calls internal services via API Gateway using service credentials.

Net result:
- Widget code never sees the user JWT.
- Widget code cannot directly call internal endpoints.

Notes:
- WSTs inheriting the caller role keeps authorization logic consistent with existing “role-gated endpoints”.
- Role inheritance is **not sufficient** on its own. Scopes still matter to prevent a widget (especially unverified) from using the full power of the user’s role.

### 6.6 Permissions model for widgets

Define explicit widget capability scopes, similar to OAuth scopes.

Examples:
- `ers.devices.read`
- `ers.rooms.read`
- `automation.runs.read`
- `automation.runs.create`
- `hdp.commands.publish` (dangerous; likely admin-only)

Policy:
- Verified widgets can request a wider set of scopes.
- Unverified widgets default to read-only scopes unless admin explicitly grants.

Role alignment:
- WST role claim should mirror the user role at issuance time.
- The effective permission check should be: **role allows** the operation AND **scopes allow** the operation AND **integration policy** allows the operation.

### 6.7 Additional safeguards

- **Rate limiting** per widget instance (to prevent noisy widgets).
- **Audit logging** for widget-proxy operations.
- **Static analysis** in verification pipeline (e.g., disallow known dangerous APIs).
- **Network egress policy** at the container/runtime level for integration containers.

---

## 7. Widget Runtime (frontend architecture)

### 7.1 Goals

- Render dashboards from server-provided layout.
- Provide “edit mode” with drag-and-drop.
- Provide generic widget settings UI.
- Ensure per-widget isolation and graceful failure.

### 7.2 Layout engine choice: Masonry (responsive)

Your preference: masonry is okay.

Mobile constraints:
- Must behave well across varying screen sizes.
- Avoid relying on many different widget widths.

Recommendation:
- Use a proven layout library rather than a custom placeholder implementation.

Recommended library for v1:
- `react-grid-layout` (Responsive + draggable layout)
  - Pros: mature, supports drag-and-drop and resizing, responsive breakpoints, widely used for dashboard UIs.
  - Mobile: configure breakpoints so small screens collapse to a single column (avoids “weird widths” across device sizes).

Design constraints (to keep UX sane across devices):
- Prefer **size hints** (`sm|md|lg`) mapped to constrained sizes per breakpoint.
- Avoid arbitrary user-defined widths on small breakpoints.

Alternative (if you want true masonry flow + custom drag logic):
- `react-masonry-css` for layout + `dnd-kit` for drag-and-drop (more engineering, more control).

### 7.3 Widget shell (“chrome”)

Every widget renders inside a standard wrapper that handles:

- Title + optional subtitle
- Loading state
- Error boundary
- 401/403 mapping:
  - 401 → show NoPermissionWidget + “Sign in”
  - 403 → show NoPermissionWidget, no login CTA

### 7.4 Add widget flow

- “Add widget” opens a tray/panel listing widget types from catalog.
- Clicking adds a widget instance to the dashboard.

### 7.5 Edit mode

- Toggle “Edit dashboard”.
- In edit mode:
  - tiles are draggable
  - remove buttons appear
  - settings buttons appear

### 7.6 Widget registry

Frontend maintains a registry:

- `widget_type` → React component

For third-party:
- `widget_type` maps to an iframe widget renderer.

---

## 8. Backend APIs (dashboard + catalog + integrations)

All endpoints are served through API Gateway and protected with JWT/roles.

### 8.1 dashboard-service API

**User dashboard**:
- `GET /api/dashboard/me`
  - returns user dashboard; clones default if absent
- `PUT /api/dashboard/me`
  - replaces dashboard items/layout
  - requires `layout_version` match

**Widget operations**:
- `POST /api/dashboard/me/widgets`
  - body: `{ widget_type, initial_layout?, initial_settings? }`
- `PATCH /api/dashboard/me/widgets/{instance_id}`
  - updates settings/layout/enable/title
- `DELETE /api/dashboard/me/widgets/{instance_id}`

**Default template (admin)**:
- `GET /api/dashboard/default`
- `PUT /api/dashboard/default`

### 8.2 Widget catalog API

- `GET /api/widgets/catalog`
  - returns all widget types available in this deployment
  - includes:
    - `verified` flag
    - `source`: `first_party` | `integration` | `local_unverified`

### 8.3 integration-registry-service API

Admin-only:
- `GET /api/integrations`
- `POST /api/integrations/install`
- `POST /api/integrations/{id}/enable`
- `POST /api/integrations/{id}/disable`
- `POST /api/integrations/{id}/update`
- `DELETE /api/integrations/{id}`
- `GET /api/integrations/{id}/manifest`
- `POST /api/integrations/{id}/config`

Resident read-only (optional):
- `GET /api/integrations/catalog`
  - sanitized; exposes only what frontend needs to show widgets and integration tabs

---

## 9. Integrations (discovery + manifests)

### 9.1 Integration manifest contract

Each integration must expose:

- `GET /.well-known/homenavi-integration.json`

Example schema (conceptual):

```json
{
  "schema_version": 1,
  "id": "spotify",
  "name": "Spotify",
  "version": "1.2.3",
  "publisher": "Example Inc",
  "homepage": "https://example.com",
  "verified": false,

  "ui": {
    "sidebar": {
      "enabled": true,
      "path": "/integrations/spotify",
      "label": "Spotify",
      "icon": "music"
    }
  },

  "widgets": [
    {
      "type": "integration.spotify.player",
      "display_name": "Spotify Player",
      "description": "Control playback.",
      "entry": {
        "kind": "iframe",
        "url": "/integrations/spotify/widgets/player/index.html"
      },
      "settings_schema": {
        "type": "object",
        "properties": {
          "default_device": {"type": "string"}
        }
      },
      "requested_scopes": ["integration.spotify.read", "integration.spotify.control"]
    }
  ],

  "automation": {
    "actions": [
      {
        "id": "spotify.play",
        "display_name": "Play",
        "input_schema": {"type": "object", "properties": {"uri": {"type": "string"}}},
        "requested_scopes": ["integration.spotify.control"]
      }
    ],
    "triggers": []
  },

  "config_schema": {
    "type": "object",
    "properties": {
      "client_id": {"type": "string"},
      "client_secret": {"type": "string"}
    },
    "required": ["client_id", "client_secret"]
  }
}
```

### 9.2 Discovery rules

integration-registry-service:

- Polls manifests on startup and periodically.
- Validates schema version.
- Stores manifest and derived catalog entries in DB.

### 9.3 Integration UI surfaces

Integrations may contribute a UI tab.

Two implementation options:

- **Iframe tab**: easiest and consistent with widget sandboxing.
- **Native tab**: only for first-party integrations shipped with frontend.

Recommendation for third-party: iframe tabs with the same sandbox constraints as widgets.

---

## 10. Automation Extensions from Integrations

### 10.1 MVP approach: generic “Integration Action” node

Implement in automation-service:

- A node type that references `{ integration_id, action_id }`.
- At runtime automation-service calls integration endpoint via gateway.

### 10.2 Next: manifest-driven UI forms

Automation UI:

- Reads `input_schema` from the integration catalog.
- Generates a form.

Automation service:

- Validates payload against schema.
- Enforces scopes/roles.

### 10.3 Security

- Automation executions must be auditable.
- Dangerous capabilities (device commands) are admin-only unless explicitly delegated.

---

## 11. Marketplace (store.homenavi.org) + Verification Pipeline

### 11.1 Marketplace responsibilities

`store.homenavi.org` provides:

- Listing integrations by id
- Version metadata
- Release assets (bundles or container references)
- Signatures and verification status
- Security advisories / revocations

### 11.2 Release format options

Option A (preferred): container images
- Integration is shipped as a container image.
- Local deployment pulls from registry.
- Pros: consistent runtime isolation.
- Cons: requires container infra.

Option B: signed widget bundles
- JS/HTML bundles signed and served.
- Local deployment downloads and serves them.
- Pros: lighter.
- Cons: harder to isolate without iframes + strict CSP.

### 11.3 Verification pipeline

Verified badge should mean:

- Source/release is known and signed.
- Automated analysis passes.
- Manual review performed.
- Integration declares scopes and follows rules.

Suggested pipeline steps:

1) Static scanning
- dependency vulnerability scan
- secrets scan
- banlist patterns (token exfil patterns)

2) Build & run
- build container
- run unit tests

3) Dynamic checks
- run in sandbox env
- ensure widget cannot access parent origin storage

4) Signing
- store signs release metadata with a root key

5) Revocation mechanism
- store can mark a release as revoked
- local deployments should refuse new installs and optionally disable

### 11.4 Verified tag UI/UX

In widget catalog and integration list:

- Show badge for verified.
- Show warning for unverified.
- Require explicit admin confirmation for unverified install.

---

## 12. Admin Panel Roadmap (integrations + ecosystem control)

### 12.1 Integrations admin (lower risk)

Features:
- Install/update/enable/disable/remove integrations
- Configure integration secrets
- View status/health

### 12.2 Ecosystem control plane (high risk)

Goals:
- Restart services
- Enable/disable adapters (zigbee, thread, etc.)
- Update to newer versions while keeping configs

Strict constraints:
- No arbitrary command execution
- Allowlist actions only
- Admin-only with audit logging

Implementation approaches:

- Docker Compose-based control:
  - manage compose overrides
  - apply `docker compose up -d` for chosen services
  - keep persistent volumes/configs

---

## 13. Phased Delivery Plan (milestones + acceptance criteria)

### Milestone 1 — Dashboard MVP (first shipping slice)

Deliver:
- dashboard-service with Postgres schema
- `GET/PUT /api/dashboard/me`
- default dashboard cloning
- frontend Home renders dashboard from API
- convert existing home cards into first-party widgets
- per-widget 401/403 fallback to NoPermissionWidget

Acceptance criteria:
- user can reorder widgets and refresh
- forbidden widget shows fallback, not page failure

### Milestone 2 — Add/Remove + Settings

Deliver:
- widget catalog endpoint
- add widget tray
- settings panel with schema-driven UI

Acceptance:
- users can add widgets and configure them

### Milestone 3 — Integration Registry (local)

Deliver:
- integration-registry-service
- manifest validation
- surface integration widgets into catalog

Acceptance:
- an installed integration contributes widgets to Home

### Milestone 4 — Sandboxed Third-Party Widgets

Deliver:
- iframe widget renderer
- widget-proxy with scoped tokens
- host-mediated data access

Acceptance:
- third-party widget cannot access user JWT

### Milestone 5 — Marketplace (central)

Deliver:
- store APIs
- signed releases
- verified badges
- local install/update flows

Acceptance:
- admin can install a verified integration from store

### Milestone 6 — Automation Extensions

Deliver:
- integration actions in automation-service
- UI generated from schemas

Acceptance:
- integrations add automation actions

### Milestone 7 — Admin Control Plane

Deliver:
- service enable/disable
- restart/update
- audit logs

Acceptance:
- admin can disable unused adapters safely

---

## 14. Testing Strategy

- Unit tests for dashboard-service schema + version concurrency.
- Frontend component tests for:
  - drag/drop persistence
  - 401/403 widget fallback
- Security tests:
  - verify iframe widgets cannot read storage
  - verify widget-proxy scope enforcement

---

## 15. Operational Considerations (deploy, updates, backups)

- DB migrations for dashboard-service and integration-registry.
- Backups:
  - Postgres backups (dashboards + integration configs)
- Update strategy:
  - versioned manifests
  - compatibility matrix for widget runtime schema_version

---

## 16. Third-Party Developer Documentation Plan

Create documentation that explains how to write integrations in any language (Go/Python preferred).

### 16.1 Required docs

- Manifest format and schema rules
- Widget development guide:
  - iframe widget contract
  - postMessage API
  - settings schema
- Automation extension guide:
  - action/trigger schema
  - runtime execution expectations
- Packaging & publishing:
  - container image requirements
  - signing requirements for verified

### 16.2 Tooling recommendation

Provide a minimal SDK:
- Go package for manifest serving + schema validation
- Python helper library for the same

---

## 17. Open Questions / Decisions

These decisions can be finalized during Milestone 0 (contract lock):

- Masonry engine choice (library vs custom).
- Whether resizing is allowed on mobile (likely limited).
- Whether integrations can ship both widgets and sidebar tabs in v1.
- Token lifetime, scope list, and audit log schema.
