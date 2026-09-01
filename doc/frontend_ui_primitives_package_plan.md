# Homenavi UI Primitives Package Plan

## Goal

Create a reusable UI package based on Homenavi's current visual language, focused on atomic and primitive components. The package should reduce duplication, simplify the current frontend, and be usable by future projects without pulling in Homenavi business logic.

## Why This Direction

Homenavi already has a partial separation between feature components and shared UI primitives. The main frontend reuses shared components from `frontend/src/components/common`, and the visual system is already grounded in shared CSS variables from `frontend/src/colors.css`.

That means the right next step is not a broad "shared frontend package". The right step is a small design-system-style package that holds:

- design tokens
- base styles
- atomic and primitive components

This matches the usual enterprise pattern more closely than extracting large shared feature components. In mature teams, the standard is typically:

- tokens as the source of truth
- dumb, prop-driven primitives
- feature and business logic kept in the app
- explicit dependency boundaries that prevent UI packages from importing app code

## Scope

### In scope

- design tokens
- global style foundations
- atomic and primitive UI components
- small composable building blocks with minimal logic
- documentation and usage examples

### Out of scope

- pages
- feature-level components
- auth-aware or router-aware components
- device- or integration-specific renderers
- data fetching, context wiring, or business rules

## Proposed Package Shape

Start with a small internal package rather than a broad cross-project framework.

```text
packages/
  ui/
    src/
      tokens/
      styles/
      components/
        BaseModal/
        Button/
        GlassCard/
        GlassPill/
        GlassSelect/
        GlassSwitch/
        ProgressBar/
        SearchBar/
    package.json
```

Keep three layers separate:

1. Design tokens: colors, spacing, radius, shadows, motion, typography.
2. Primitives: button, card, switch, pill, modal shell, input/select wrappers.
3. App/domain components: auth widgets, page headers with navigation, device renderers, profile flows.

## Candidate Classification

### Good extraction candidates

- `GlassCard`
- `Button`
- `GlassPill`
- `GlassSwitch`
- `BaseModal`
- `GlassSelect`
- `SearchBar`
- `ProgressBar`
- low-level avatar, badge, or shell primitives if their APIs stay generic

### Needs refactor before extraction

- `PageHeader`
- `UnauthorizedView`
- `NoPermissionWidget`

### Should stay in the app

- `ProfileButton`
- `DeviceControlRenderer`
- `DeviceMetricsRenderer`
- `IntegrationCard` if it stays domain-shaped around integrations

## Extraction Rules

Any component moved into the package must satisfy all of these rules:

- no imports from app services
- no imports from app auth or feature context
- no router dependency unless intentionally abstracted behind props
- no Homenavi-specific custom browser events
- no direct dependency on device or integration domain formatting rules
- styling must depend on package tokens, not scattered hardcoded values

If a component violates one of these rules, it either needs a refactor first or it should remain in the app.

## Execution Plan

### Phase 1: Audit and freeze the extraction list

- review current shared components under `frontend/src/components/common`
- classify each component as generic primitive, app-aware shared UI, or domain-specific shared UI
- define the v0 extraction set before moving files
- avoid scope creep by explicitly excluding anything that mixes navigation, auth, or domain formatting

Deliverable:
- a locked first-pass list of components that will move into the package

### Phase 2: Extract design tokens first

- move the `frontend/src/colors.css` system into the package as the canonical token source
- normalize naming where needed so tokens represent semantics, not one-off use cases
- add any missing token categories currently hardcoded in components
- keep the token layer framework-agnostic and CSS-variable-first

Deliverable:
- a reusable token and foundation stylesheet that can be consumed independently of the components

### Phase 3: Extract primitive components

- move the selected primitive components into the package
- keep their APIs generic and prop-driven
- replace Homenavi-specific behavior with callbacks or explicit props
- preserve current visual output while simplifying dependency boundaries

Examples:
- `PageHeader` should accept `onBack` and `showBack`, but should not know about router navigation
- `NoPermissionWidget` should accept an `onLoginClick` callback instead of dispatching a Homenavi event

Deliverable:
- a small reusable component set with no app-code imports

### Phase 4: Add package ergonomics

- add a build setup for the UI package
- declare React as a peer dependency
- expose a clear public API for components and styles
- document the expected CSS import path and consumption model
- integrate local consumption through workspace linking or monorepo package references

Deliverable:
- a consumable internal UI package wired into the repository

### Phase 5: Migrate Homenavi to consume the package

- replace local imports incrementally rather than all at once
- start with the safest primitives:
  - `Button`
  - `GlassCard`
  - `GlassPill`
  - `GlassSwitch`
- validate styling after each migration slice
- leave app-aware components local until their contracts are cleaned up

Deliverable:
- Homenavi consuming the package for its low-level building blocks

### Phase 6: Add documentation and guardrails

- add a package README with install and usage examples
- document contribution rules for what belongs in the package
- enforce boundaries socially first, then with lint or import restrictions if needed
- define a review rule that primitives must not import app code

Deliverable:
- a package that stays clean instead of regressing into another shared misc folder

## Acceptance Criteria

The package is successful when all of the following are true:

- primitives can be imported into another React app without bringing Homenavi auth, services, or routing assumptions
- tokens define the shared visual language in one place
- Homenavi consumes the package for low-level building blocks
- package consumers can compose feature UIs without depending on Homenavi business code

## Risks

- extracting too many components too early will make the package messy and hard to reuse
- mixing generic primitives with domain-specific shared UI will weaken the boundary immediately
- trying to solve Tailwind and non-Tailwind consumption in v1 will slow the effort down unnecessarily
- keeping current app assumptions in component APIs will make the package reusable only in name

## Recommended First Milestone

Build a v0 package with:

- tokens
- `Button`
- `GlassCard`
- `GlassPill`
- `GlassSwitch`
- `BaseModal`
- `SearchBar`

Do not extract pages, auth flows, dashboard widgets, or device renderers in the first round.

## Expected Effort

### Initial v0 extraction

Around 2 to 5 days.

### Polished internal package used by Homenavi

Around 1 to 2 weeks.

### Broader design-system package ready for multiple unrelated projects

Around 2 to 4 weeks.

## Practical Notes For This Repo

- The current separation is good enough to start, but not clean enough to move the entire `common` folder as-is.
- The package boundary should be introduced by extracting primitives first, not by rebranding shared feature components.
- The marketplace frontend currently uses a different styling model, so v1 should optimize for the main Homenavi frontend rather than trying to unify all frontends immediately.
- The main simplification benefit inside Homenavi will come from reducing duplicated styling and making shared primitive APIs stricter.

## Next Step After This Plan

The best implementation next step is to produce a component extraction matrix for everything under `frontend/src/components/common`, with each item marked as one of:

- extract now
- refactor first
- keep local

That matrix should be used to scope the first package iteration before any code is moved.