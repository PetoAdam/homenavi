# Shared Packages Migration Plan

## Goal

Create a reusable shared Go package layer for common infrastructure concerns in the monorepo, then migrate services incrementally without breaking local development, Docker Compose builds, or Helm-based deployment.

## Scope for the first migration wave

Foundation packages:
- `shared/envx` — environment/config parsing helpers
- `shared/hdp` — HDP topic/envelope constants and helpers
- `shared/mqttx` — reusable MQTT client wrapper over Paho
- `shared/dbx` — shared Postgres DSN helpers

First service migration wave:
- `history-service`
- `automation-service`
- `entity-registry-service`

## Constraints discovered in current code

- Services are separate Go modules today.
- Several services build Docker images from a service-local build context, which prevents importing an unpublished sibling module unless build context is widened.
- MQTT, HDP topic constants, and env parsing are duplicated across multiple services.

## Milestones

### Milestone 1 — Foundation
- Add root `go.work`
- Add `shared` Go module with unit tests
- Add a deployment validation script for:
	- shared package tests
	- migrated service tests
	- Docker Compose config render
	- Helm lint/template render

### Milestone 2 — First adopters
- Migrate `history-service` to shared `envx`, `hdp`, `mqttx`
- Migrate `automation-service` to shared `envx`, `mqttx`, `hdp`
- Migrate `entity-registry-service` to shared `envx`, `mqttx`, `hdp`
- Rename migrated service modules to fully qualified import paths

### Milestone 3 — Device plane
- Migrate `device-hub`, `zigbee-adapter`, `thread-adapter` to shared `hdp` and `mqttx`
- Standardize retained publish semantics behind shared APIs

### Milestone 4 — Broader consolidation
- Migrate remaining services to fully qualified module paths
- Extract more shared packages only where duplication is stable and proven
- Evaluate follow-up packages for auth, observability, and db bootstrap

## Test strategy

### Unit tests
- `go test ./shared/...`
- Existing per-service unit tests for each migrated service

### Local development validation
- `docker compose config` from repo root
- Migrated service Docker builds from repo root context

### Deployment validation
- `helm lint ./helm/homenavi`
- `helm template homenavi ./helm/homenavi`

## Migration rules

- Prefer thin shared helpers first, not giant framework packages.
- Keep service-specific config structs and repositories local.
- Migrate in waves so Docker/Compose/Helm remain working after every step.
- Do not couple the browser realtime layer to the new shared Go packages.

## Success criteria for this first step

- Shared packages compile and are covered by tests.
- At least one migration wave of real services imports the shared packages.
- Docker Compose and Helm validation remain green.
- Migrated services use fully qualified module paths.
