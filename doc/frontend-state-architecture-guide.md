# Frontend State Architecture Guide

## Purpose

This guide documents the target frontend state model after the state-management migration.

Use it when adding new UI flows, refactoring old screens, or reviewing pull requests.

## Layer selection

### 1. React Query

Use React Query for server-owned state:
- collections loaded from APIs
- entity detail payloads
- mutation lifecycle and invalidation
- stale-time, retries, and refetch policy

Examples in this codebase:
- integrations registry and marketplace hooks
- dashboard document and widget catalog loading
- ERS inventory loading

Rules:
- keep transport details in `frontend/src/services/*`
- keep cache keys in `frontend/src/state/queryKeys.js`
- keep invalidation policy in query or mutation hooks, not in page components
- prefer query helpers that preserve current component-facing shapes when migrating old code

### 2. Zustand

Use Zustand for client-owned state that should survive navigation:
- view preferences
- filters and sorting preferences
- panel modes that are shared across screens

Examples in this codebase:
- devices view preferences in `frontend/src/state/devicesViewStore.js`

Rules:
- store only durable client state here
- avoid putting request lifecycle state into Zustand
- expose small selector-based reads to keep renders narrow

### 3. Reducers

Use reducers for complex local workflow state:
- multi-step flows
- modal stacks
- explicit transient transitions
- pending/error/reset logic that belongs to one feature slice

Examples in this codebase:
- device detail UI/history reducers
- groups editor reducer
- automation UI reducer
- dashboard UI reducer
- map controller UI reducer
- add-device modal reducer

Rules:
- model transitions as named actions
- prefer one reducer per tightly coupled workflow slice
- add focused reducer tests for each new reducer

## Preferred module boundaries

### Services

`frontend/src/services/*` should stay thin.
They should only translate between the frontend and backend transport APIs.

### Hooks

Feature hooks should own:
- query composition
- mutation orchestration
- invalidation and refetch behavior
- optimistic updates when needed
- normalization required by multiple UI consumers

### Components

Components should prefer:
- rendering
- local interaction wiring
- dispatching reducer actions
- consuming feature hooks

Avoid adding fresh page-level service orchestration inside large components when a dedicated hook can own it.

## Migration-era patterns that are now preferred

Prefer:
- query helper + custom hook for remote data
- reducer for coupled transient state
- Zustand for persistent view settings

Avoid:
- repeating loading/error booleans in page components for the same API domain
- manual cache invalidation scattered across components
- large components with many unrelated `useState` branches for one workflow

## Testing expectations

When adding or changing state logic:
- add reducer transition tests for new reducers
- add focused tests for query key factories and query helper functions
- add focused tests for mutation reconciliation logic when it contains branchy behavior
- keep the full frontend suite green

## Review standard

Before merging state-management changes, verify:
- the chosen layer matches ownership of the state
- transport calls are not leaking back into large page components without a good reason
- query invalidation and optimistic updates are explicit
- reducers have action-level tests
- lint and frontend tests pass
