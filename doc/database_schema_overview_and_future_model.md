# Homenavi Database Schema

This document describes the steady-state PostgreSQL schema for `homenavi`.

## Schema principles

- UUID is the canonical key type for relational references.
- GORM model definitions in each service are the schema authority.
- Legacy HDP table names are not part of the steady-state schema.
- HDP runtime addressing still uses packed `protocol/external_id` strings where MQTT topics require them.
- Relational joins that cross into HDP use typed `uuid` bridge columns.
- ERS and HDP stay as separate domains in the database.

## Service-owned table map

### Dashboard service

- `dashboards`

### Device hub

- `hdp_devices`
- `hdp_device_states`

### History service

- `hdp_device_state_points`

### Entity registry service

- `ers_rooms`
- `ers_tags`
- `ers_groups`
- `ers_devices`
- `ers_device_tags`
- `ers_group_members`
- `ers_device_bindings`

### Automation service

- `workflows`
- `workflow_runs`
- `workflow_run_steps`
- `pending_correlations`

### Auth service

- `user_rows`
- `email_verification_rows`

## Canonical HDP tables

### `hdp_devices`

Purpose: canonical HDP device registry keyed by UUID.

Columns:

| Column | Type | Null | Notes |
| --- | --- | --- | --- |
| `id` | `uuid` | no | primary key |
| `protocol` | `text` | no | adapter namespace |
| `external_id` | `text` | no | adapter-local identifier |
| `name` | `text` | yes | optional display name |
| `type` | `text` | yes | optional device type |
| `manufacturer` | `text` | yes | optional vendor metadata |
| `model` | `text` | yes | optional model metadata |
| `description` | `text` | yes | optional long description |
| `firmware` | `text` | yes | optional firmware string |
| `icon` | `text` | yes | optional icon id |
| `capabilities` | `jsonb` | no | defaults to `[]` |
| `inputs` | `jsonb` | no | defaults to `[]` |
| `online` | `boolean` | no | defaults to `false` |
| `last_seen` | `timestamptz` | yes | most recent heartbeat/state timestamp |
| `created_at` | `timestamptz` | yes | standard GORM timestamp |
| `updated_at` | `timestamptz` | yes | standard GORM timestamp |

Indexes:

- primary key on `id`
- unique index on `(protocol, external_id)`

### `hdp_device_states`

Purpose: current HDP device state snapshot.

Columns:

| Column | Type | Null | Notes |
| --- | --- | --- | --- |
| `device_id` | `uuid` | no | primary key; HDP device UUID |
| `state` | `jsonb` | no | defaults to `{}` |
| `updated_at` | `timestamptz` | yes | last persisted snapshot time |

### `hdp_device_state_points`

Purpose: append-only HDP state history.

Columns:

| Column | Type | Null | Notes |
| --- | --- | --- | --- |
| `id` | `uuid` | no | primary key |
| `device_id` | `text` | yes | runtime HDP identifier used in MQTT payloads |
| `hdp_device_id` | `uuid` | yes | typed HDP device reference for joins |
| `ts` | `timestamptz` | yes | event timestamp |
| `payload` | `jsonb` | yes | raw state envelope |
| `topic` | `text` | yes | original MQTT topic |
| `retained` | `boolean` | yes | retained-message flag |
| `ingested_at` | `timestamptz` | yes | persistence timestamp |

Indexes:

- primary key on `id`
- composite index on `(device_id, ts)`
- index on `hdp_device_id`

## Entity registry tables

### `ers_rooms`

Purpose: room catalog for human organization.

Key fields:

- `id uuid primary key`
- `slug text not null unique`
- `name text not null`
- `description text`
- `meta jsonb not null default '{}'`

### `ers_tags`

Purpose: reusable labels for selectors.

Key fields:

- `id uuid primary key`
- `slug text not null unique`
- `name text not null`
- `description text`

### `ers_groups`

Purpose: named sets of ERS devices.

Key fields:

- `id uuid primary key`
- `slug text not null unique`
- `name text not null`
- `description text`
- `meta jsonb not null default '{}'`

### `ers_devices`

Purpose: entity-registry device catalog, independent from HDP transport storage.

Key fields:

- `id uuid primary key`
- `room_id uuid nullable`
- `name text not null`
- `description text`
- `meta jsonb not null default '{}'`

Relations:

- optional `room_id -> ers_rooms.id`

### `ers_device_tags`

Purpose: many-to-many join between `ers_devices` and `ers_tags`.

Key fields:

- `device_id uuid not null`
- `tag_id uuid not null`

Relations:

- `device_id -> ers_devices.id`
- `tag_id -> ers_tags.id`

### `ers_group_members`

Purpose: many-to-many join between `ers_groups` and `ers_devices`.

Key fields:

- `group_id uuid not null`
- `device_id uuid not null`

Relations:

- `group_id -> ers_groups.id`
- `device_id -> ers_devices.id`

### `ers_device_bindings`

Purpose: maps ERS devices to external runtimes, with HDP using both runtime and typed references.

Columns:

| Column | Type | Null | Notes |
| --- | --- | --- | --- |
| `id` | `uuid` | no | primary key |
| `device_id` | `uuid` | no | ERS device reference |
| `kind` | `text` | no | binding namespace, including `hdp` |
| `external_id` | `text` | no | runtime-facing identifier |
| `hdp_device_id` | `uuid` | yes | typed HDP device UUID bridge |
| `created_at` | `timestamptz` | yes | standard GORM timestamp |
| `updated_at` | `timestamptz` | yes | standard GORM timestamp |

Indexes:

- primary key on `id`
- index on `device_id`
- index on `kind`
- index on `external_id`
- unique index on `(kind, external_id)`
- index on `hdp_device_id`

Relations:

- `device_id -> ers_devices.id`
- `hdp_device_id -> hdp_devices.id` logically, using the same UUID key type

## Automation tables

### `workflows`

Purpose: persisted automation definitions and authoring source metadata.

Columns:

| Column | Type | Null | Notes |
| --- | --- | --- | --- |
| `id` | `uuid` | no | primary key |
| `name` | `text` | no | workflow name |
| `enabled` | `boolean` | no | defaults to `false` |
| `definition` | `jsonb` | no | normalized runtime definition |
| `source_kind` | `varchar(16)` | no | defaults to `graph` |
| `source_format` | `varchar(32)` | no | defaults to `graph-json` |
| `source_code` | `text` | no | defaults to empty string |
| `source_revision` | `bigint` | no | defaults to `1` |
| `created_by` | `text` | no | user or actor id |
| `created_at` | `timestamptz` | yes | standard GORM timestamp |
| `updated_at` | `timestamptz` | yes | standard GORM timestamp |

### `workflow_runs`

Purpose: workflow execution instances.

Key fields:

- `id uuid primary key`
- `workflow_id uuid not null`
- `status text not null`
- `trigger_event jsonb`
- `error text`
- `started_at timestamptz not null`
- `finished_at timestamptz nullable`

Relation:

- `workflow_id -> workflows.id`

### `workflow_run_steps`

Purpose: per-node execution records for a workflow run.

Key fields:

- `id uuid primary key`
- `run_id uuid not null`
- `node_id text not null`
- `status text not null`
- `input jsonb`
- `output jsonb`
- `error text`
- `started_at timestamptz not null`
- `finished_at timestamptz nullable`

Relation:

- `run_id -> workflow_runs.id`

### `pending_correlations`

Purpose: tracks command-result waits during workflow execution.

Columns:

| Column | Type | Null | Notes |
| --- | --- | --- | --- |
| `corr` | `text` | no | primary key |
| `run_id` | `uuid` | no | workflow run reference |
| `workflow_id` | `uuid` | no | workflow reference |
| `device_id` | `text` | no | runtime HDP identifier used for MQTT correlation |
| `hdp_device_id` | `uuid` | yes | typed HDP device UUID bridge |
| `created_at` | `timestamptz` | no | creation time |
| `expires_at` | `timestamptz` | no | timeout boundary |

Indexes:

- primary key on `corr`
- index on `run_id`
- index on `workflow_id`
- index on `expires_at`
- index on `hdp_device_id`

Relations:

- `run_id -> workflow_runs.id`
- `workflow_id -> workflows.id`
- `hdp_device_id -> hdp_devices.id` logically, using the same UUID key type

## Dashboard and auth tables

### `dashboards`

Purpose: persisted dashboard documents.

Key fields:

- `id uuid primary key`
- `scope varchar not null`
- `owner_user_id uuid nullable`
- `title varchar not null`
- `layout_engine varchar not null`
- `layout_version int not null default 1`
- `doc jsonb`
- `created_at timestamptz not null default now()`
- `updated_at timestamptz not null default now()`

### `user_rows`

Purpose: auth-service user identity storage.

### `email_verification_rows`

Purpose: auth-service email verification state.

## Redis topology and usage

Redis is not part of the PostgreSQL schema, but it is part of the persistence and runtime consistency model for several services.

Shared connection rules:

- `shared/redisx/config.go` is the authority for Redis configuration parsing and validation.
- Supported modes are `standalone` and `sentinel`.
- `REDIS_MODE=sentinel` requires `REDIS_SENTINEL_ADDRS` and `REDIS_MASTER_NAME`.
- `REDIS_ADDR` is the standalone fallback when Sentinel addresses are not configured.
- `redisx.Connect` is the standard connector and performs a startup `PING`.
- Sentinel mode uses `redis.NewFailoverClient`, so failover behavior is centralized instead of being reimplemented per service.

Shared cache abstraction:

- `shared/cachex/json_store.go` is the standard JSON cache wrapper.
- Cache misses are normalized as `ErrCacheMiss` instead of leaking raw `redis.Nil` handling into callers.

Service usage map:

- `api-gateway` uses Redis for token-bucket rate limiting through Lua state stored in Redis hashes.
- `dashboard-service` uses Redis JSON cache entries for dashboard-serving acceleration.
- `device-hub` uses the shared JSON cache for HTTP and device-facing cached views.
- `entity-registry-service` uses the shared JSON cache for ERS API response caching.
- `auth-service` uses Redis for transient auth and verification state, and now uses the shared `redisx.Connect` path so Sentinel mode and startup validation behave the same as the rest of the stack.
- `zigbee-adapter` uses the shared Redis connector for state cache access.

Operational expectation:

- PostgreSQL is the source of truth for durable relational state.
- Redis is disposable runtime state and cache infrastructure.
- A Redis outage should degrade cache-backed paths or rate limiting, but it must not redefine the relational schema or require table-level recovery steps.

## Query model summary

- Use HDP UUID columns for joins, audits, and relational lookups.
- Use runtime `protocol/external_id` strings only where topic routing or external payloads require them.
- Resolve selectors in ERS through ERS device IDs first, then HDP UUID bridges, then runtime identifiers for API payload compatibility.
- Keep history as an append-only time-series table with both runtime and typed HDP identifiers.
