# Database Schema Ownership

This document defines how the `homenavi` PostgreSQL schema is owned and maintained from Go code.

## Authority

- GORM model definitions are the schema source of truth.
- Repository initialization code is responsible for idempotent table creation, column creation, default backfills, and compatibility-safe constraint creation.
- Each service owns only its own tables.
- Raw SQL exists only as an operator execution artifact when a direct database conversion is needed.

## Ownership by service

### Dashboard service

- owns `dashboards`
- initializes the table with explicit `gorm.Migrator()` operations

### Device hub

- owns `hdp_devices`
- owns `hdp_device_states`
- persists runtime metadata and current state
- keeps JSON defaults aligned with the Go models

### History service

- owns `hdp_device_state_points`
- persists append-only state history
- keeps the runtime `device_id` string for payload fidelity and `hdp_device_id` for typed joins

### Entity registry service

- owns `ers_rooms`
- owns `ers_tags`
- owns `ers_groups`
- owns `ers_devices`
- owns `ers_device_tags`
- owns `ers_group_members`
- owns `ers_device_bindings`
- manages ERS-to-HDP bridging with `hdp_device_id` as the typed reference

### Automation service

- owns `workflows`
- owns `workflow_runs`
- owns `workflow_run_steps`
- owns `pending_correlations`
- stores workflow source metadata directly in `workflows`
- stores both runtime `device_id` and typed `hdp_device_id` in `pending_correlations`

### Auth service

- owns `user_rows`
- owns `email_verification_rows`

## GORM rules used in this repository

- Prefer explicit `gorm.Migrator()` steps when a service has shown unstable broad `AutoMigrate` behavior against PostgreSQL.
- Use `AutoMigrate` only where the service has a stable, narrow table surface and the generated SQL is predictable.
- Always keep primary and foreign key types aligned as `uuid` when a typed relationship exists.
- Backfill new non-null JSON columns before applying `NOT NULL` or default expectations.
- Add compatibility bridge columns before routing internal code paths through them.
- Keep startup schema initialization idempotent.

## Relationship rules

- ERS and HDP are separate models and stay separate in storage.
- HDP runtime strings are transport identifiers, not relational keys.
- `ers_device_bindings.hdp_device_id`, `pending_correlations.hdp_device_id`, and `hdp_device_state_points.hdp_device_id` are the typed HDP join path.
- History remains retention-oriented and should not depend on hard live-device deletion semantics.

## How to add or change schema

1. Update the owning GORM model in the service that owns the table.
2. Update the owning repository initializer to create or backfill the change idempotently.
3. Update focused tests for the owning package.
4. Validate against local PostgreSQL through the compose stack, not only in-memory tests.
5. If operators need a one-shot production conversion, derive the SQL artifact from the final code-owned schema rather than treating the SQL as the source of truth.

## Operational expectation

- A service must be able to start repeatedly against an already-initialized database without logging duplicate DDL failures.
- Local compose validation is required for schema-affecting work.
- Schema documentation should describe the steady-state tables and ownership model, not historical intermediate layouts.

## Redis support model

- Redis configuration is shared through `shared/redisx/config.go`.
- Services should use `redisx.Connect` instead of constructing clients directly so Sentinel failover support and startup `PING` behavior stay consistent.
- `shared/cachex/json_store.go` is the preferred cache wrapper for JSON payload caches.
- Current Redis-backed services are `api-gateway`, `dashboard-service`, `device-hub`, `entity-registry-service`, `auth-service`, and `zigbee-adapter`.
- Redis is operational infrastructure, not schema authority: no Redis key layout should be treated as equivalent to a database migration contract.
