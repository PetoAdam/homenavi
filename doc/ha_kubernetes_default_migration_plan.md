# Homenavi HA-by-Default Kubernetes Migration Plan

Status: Proposed  
Date: 2026-04-13

---

## 1. Executive summary

Homenavi should keep **Docker Compose simple** and make **Helm/Kubernetes the HA-oriented default**.

That means:

- replace Mosquitto with **EMQX** as the default MQTT broker
- stop treating profile pictures as shared local files and move to **S3-compatible object storage**
- use **Redis standalone** for Docker and **Redis + Sentinel** for Kubernetes
- use **single PostgreSQL** for Docker and **CloudNativePG** for Kubernetes
- expose clean application-level abstractions so runtime wiring is configurable without rewriting services
- avoid Bitnami images/charts in the default stack
- allow advanced users to disable bundled dependencies and point Homenavi at their own charts or managed services

This plan does **not** try to make Docker Compose fully HA. Compose remains the low-friction deployment mode. The HA work is primarily aimed at Helm/Kubernetes.

---

## 2. Why this change is needed

Current state in the repo is functional, but not HA-oriented:

- Helm and Compose both deploy **Mosquitto** as a single broker.
- Profile pictures are served from a shared `/uploads` directory backed by a PVC or Compose volume.
- Helm currently deploys **single-instance Postgres** and **single-instance Redis**.
- Nginx serves profile pictures from a mounted volume, which tightly couples file serving to pod-local or PVC-local storage.

Those defaults are acceptable for local and hobby setups, but they are poor defaults for Kubernetes where users reasonably expect:

- easier failover
- fewer single-writer PVC bottlenecks
- cleaner scaling semantics
- more standard cloud/homelab storage patterns

---

## 3. Design principles

1. **Different defaults for different runtimes**
	 - Docker Compose: simple, single-node, low-ops
	 - Helm/Kubernetes: resilient by default

2. **Application contracts first**
	 - Services should depend on abstractions like `storage`, `redis`, `postgres`, and `mqtt`, not on one specific product.

3. **No Bitnami dependency**
	 - Prefer upstream/vendor-maintained images, operators, and charts.

4. **Bring-your-own-infrastructure friendly**
	 - Power users must be able to disable bundled dependencies and connect Homenavi to existing Redis, PostgreSQL, S3, or MQTT infrastructure.

5. **Incremental migration**
	 - Move one dependency boundary at a time.
	 - Preserve backward compatibility where practical.

---

## 4. Target deployment model

## 4.1 Docker Compose default

Compose should remain operationally simple:

- **EMQX single-node**
- **PostgreSQL single instance**
- **Redis single instance**
- **MinIO single-node**
- local Nginx / frontend / services as today

This gives Docker users the same functional contracts as Kubernetes without pretending to offer real HA.

## 4.2 Helm/Kubernetes default

Helm should become the HA-oriented default:

- **EMQX** for MQTT
- **CloudNativePG** for PostgreSQL
- **Redis + Sentinel** for cache, rate limits, lockouts, and ephemeral coordination
- **MinIO** for S3-compatible object storage
- bundled Homenavi services remain largely stateless

This is the default platform stance for Kubernetes.

---

## 5. Recommended dependency choices

## 5.1 MQTT: EMQX instead of Mosquitto

### Why

EMQX is a better default for Homenavi's Kubernetes path because it supports:

- clustering and better horizontal growth options
- stronger operational features
- good MQTT-over-WebSocket support
- better long-term fit for device and event traffic

### Default mode by runtime

- Compose: single EMQX node
- Helm: EMQX chart or EMQX-backed dependency mode, scalable later

### Migration note

Application code should not care whether the broker is EMQX or Mosquitto as long as `MQTT_BROKER_URL` and WebSocket routing stay stable.

---

## 5.2 Object storage: MinIO + S3 instead of shared upload PVCs

### Why

Profile pictures are the clearest current misuse of persistent shared filesystem state.

PVC-backed uploads create several problems:

- awkward scaling for frontend/profile-picture-service
- tight coupling between storage and pod placement
- hard migration path to multi-node clusters
- Nginx becomes responsible for serving app-owned binary objects

### Target model

Move profile pictures to S3-compatible object storage:

- Compose: single-node MinIO
- Helm: MinIO by default, with option to use external S3-compatible storage

### Access model

Use **pre-signed URLs** instead of directly exposing `/uploads/...` from a shared volume.

Recommended flow:

1. Client asks backend for an upload URL.
2. Backend returns pre-signed `PUT` or `POST` URL plus object key.
3. Client uploads directly to object storage.
4. Backend stores only object metadata/key.
5. Client retrieves an avatar via pre-signed `GET` URL or stable proxy endpoint that redirects/streams from object storage.

### Result

- no shared upload PVC requirement
- easier pod rescheduling
- cleaner CDN/caching story later
- easier use of external S3 providers

---

## 5.3 Redis: standalone for Docker, Sentinel for Kubernetes

### Why

Redis is used for ephemeral platform concerns such as:

- rate limits
- lockouts
- token/session-adjacent caching
- short-lived coordination

For Docker, standalone Redis is fine. For Kubernetes, Redis should fail over automatically.

### Target model

- Compose: `redis.mode=standalone`
- Helm: `redis.mode=sentinel`

### Recommendation

Do **not** hard-code `REDIS_ADDR=redis:6379` as the only contract.

Support both:

- direct address mode
- Sentinel discovery mode

---

## 5.4 PostgreSQL: standalone for Docker, CloudNativePG for Kubernetes

### Why

CloudNativePG is a strong Kubernetes-native choice for PostgreSQL HA, backups, and lifecycle management.

### Target model

- Compose: single PostgreSQL container
- Helm: CloudNativePG cluster as the default database backing service

### Recommendation

Application services should consume a stable database contract:

- host
- port
- database
- username
- password
- ssl mode if needed later

Services should not depend on whether the backing database is plain Postgres, CloudNativePG, or an external managed database.

---

## 6. Required application abstractions

The key migration rule is: **product choices belong in deployment config, not in service logic**.

## 6.1 Storage abstraction

Recommended values structure:

```yaml
storage:
	type: s3 # s3 | local
	s3:
		endpoint: http://minio:9000
		region: us-east-1
		bucket: homenavi-assets
		accessKeySecretRef: homenavi-storage
		secretKeySecretRef: homenavi-storage
		forcePathStyle: true
		presignExpirySeconds: 900
	local:
		uploadDir: /uploads
```

Rules:

- `local` is for local/dev fallback only
- `s3` is the preferred mode for production and Helm
- the profile picture service should depend on a storage interface, not directly on `/uploads`

## 6.2 Redis abstraction

Recommended values structure:

```yaml
redis:
	mode: sentinel # standalone | sentinel
	standalone:
		host: redis
		port: 6379
	sentinel:
		masterName: homenavi-redis
		nodes:
			- redis-sentinel-0.redis-sentinel:26379
			- redis-sentinel-1.redis-sentinel:26379
			- redis-sentinel-2.redis-sentinel:26379
		username: ""
		passwordSecretRef: homenavi-redis
```

Rules:

- services should initialize Redis clients based on mode
- the old single-address path can remain as a compatibility fallback during migration

## 6.3 PostgreSQL abstraction

Recommended values structure:

```yaml
postgres:
	mode: external # external | cnpgManaged
	host: homenavi-postgres-rw
	port: 5432
	database: users
	username: user
	passwordSecretRef: homenavi-postgres
	sslMode: disable
```

Rules:

- `host` remains the main application contract
- `cnpgManaged` is a Helm deployment concern, not an app concern
- power users can set `mode=external` and point to any PostgreSQL-compatible endpoint

## 6.4 MQTT abstraction

Recommended values structure:

```yaml
mqtt:
	mode: external # external | bundled
	url: mqtt://emqx:1883
	websocketUrl: ws://emqx:8083/mqtt
	username: ""
	passwordSecretRef: ""
```

Rules:

- services should only care about broker URL(s) and credentials
- broker product specifics remain a deployment concern

---

## 7. Helm architecture changes

## 7.1 Stop baking every dependency directly into one fixed chart shape

The chart should distinguish between:

- **bundled dependency mode**
- **external dependency mode**

Example:

```yaml
dependencies:
	mqtt:
		provider: emqx # emqx | external
	objectStorage:
		provider: minio # minio | external
	redis:
		provider: sentinel # standalone | sentinel | external
	postgres:
		provider: cnpg # standalone | cnpg | external
```

This makes the default Kubernetes install opinionated, while still letting advanced operators opt out.

## 7.2 Recommended Helm patterns

### EMQX

- default provider: bundled EMQX
- external mode: user supplies `mqtt.url` and disables bundled broker

### MinIO

- default provider: bundled MinIO
- external mode: user supplies S3 endpoint/bucket/credentials

### Redis

- default provider: bundled Redis + Sentinel manifests or subchart
- external mode: user supplies Sentinel endpoints or standalone host

### PostgreSQL

- default provider: bundled CloudNativePG cluster definition
- external mode: user supplies normal `postgres.host` contract

---

## 8. Bring-your-own-chart / power-user strategy

Power users should be able to use their own charts without forking Homenavi.

## 8.1 Contract

Homenavi should support this cleanly:

- disable bundled dependency deployment
- point services at externally managed endpoints
- optionally reference existing Kubernetes secrets
- keep all Homenavi app charts installable without owning the dependency lifecycle

## 8.2 Example values for externalized infrastructure

```yaml
dependencies:
	mqtt:
		provider: external
	objectStorage:
		provider: external
	redis:
		provider: external
	postgres:
		provider: external

mqtt:
	url: mqtt://my-emqx-listener.default.svc:1883
	websocketUrl: wss://mqtt.example.com/mqtt

storage:
	type: s3
	s3:
		endpoint: https://s3.example.internal
		bucket: homenavi-assets
		accessKeySecretRef: external-storage
		secretKeySecretRef: external-storage

redis:
	mode: sentinel
	sentinel:
		masterName: redis-prod
		nodes:
			- redis-sentinel-0.redis.svc:26379
			- redis-sentinel-1.redis.svc:26379
			- redis-sentinel-2.redis.svc:26379
		passwordSecretRef: external-redis

postgres:
	mode: external
	host: app-postgres-rw.database.svc
	port: 5432
	database: homenavi
	username: homenavi
	passwordSecretRef: external-postgres
```

## 8.3 What Homenavi should not do

Homenavi should not assume:

- it owns the dependency namespace
- it owns the dependency chart release names
- it owns backups for externally managed systems

That separation is important for advanced homelab and production users.

---

## 9. Profile picture migration plan

This is the most important application-level migration.

## Phase 1: introduce storage abstraction

- create a storage interface in the profile picture service
- support `local` and `s3` backends
- keep existing `/uploads` behavior temporarily for compatibility

## Phase 2: change API contract

Add API patterns like:

- `POST /profile-pictures/upload-url`
- `POST /profile-pictures/complete`
- `GET /profile-pictures/{id}/access-url`

or equivalent auth-service mediated endpoints.

The important part is that the backend returns object-storage URLs instead of filesystem paths.

## Phase 3: stop using Nginx shared volume for avatars

- remove `profile-pictures` shared volume from frontend/Nginx path
- stop exposing `/uploads/...` as the primary storage path
- optionally keep a compatibility redirect layer during transition

## Phase 4: data migration

- scan existing files from current upload volume
- upload them to MinIO/S3 under stable object keys
- update database references if needed
- keep old volume read-only for rollback window

---

## 10. MQTT migration plan

## Phase 1: make broker host configurable everywhere

- replace hard-coded Mosquitto assumptions in docs and chart values
- standardize on generic `mqtt.url` and `mqtt.websocketUrl`

## Phase 2: deploy EMQX in Compose and Helm

- Compose: replace Mosquitto service with EMQX single-node
- Helm: replace bundled Mosquitto with EMQX-backed deployment mode

## Phase 3: update gateway and frontend WS assumptions

- keep `/ws/hdp` external behavior stable
- retarget backend WebSocket upstream to EMQX

## Phase 4: retire Mosquitto-specific docs

- remove bridge examples that are specific to Mosquitto config syntax
- add EMQX bridge/cluster guidance separately if still needed

---

## 11. Redis and PostgreSQL migration plan

## Phase 1: config abstraction

- add mode-based Redis config support in services
- keep existing single-host env vars as compatibility fallback
- normalize Postgres config around `postgres.host` contract

## Phase 2: Helm dependency refactor

- add bundled/external provider toggles
- introduce CloudNativePG-backed default database mode
- introduce Redis Sentinel-backed default cache mode

## Phase 3: operational docs

- backup/restore guidance for CNPG
- failover and recovery checks for Redis Sentinel
- dependency health expectations in readiness docs

---

## 12. Backward compatibility guidance

To keep migration safe:

- keep old env vars working for at least one transition release
- map old env vars into the new config layer when possible
- keep `storage.type=local` available for development
- keep single-node Docker images and Compose examples simple

Recommended compatibility mapping examples:

- `REDIS_ADDR` -> used when `redis.mode=standalone` and no structured config exists
- `POSTGRES_HOST` -> remains valid in all modes
- `/uploads/...` -> temporary compatibility path only

---

## 13. Suggested implementation order

1. **Introduce config abstractions first**
	 - `storage`
	 - `redis`
	 - `postgres`
	 - `mqtt`

2. **Migrate profile pictures to object storage**
	 - highest HA payoff

3. **Swap Mosquitto for EMQX**
	 - lowest application risk if URLs stay generic

4. **Refactor Helm dependency model**
	 - bundled vs external providers

5. **Adopt CloudNativePG and Redis Sentinel in Helm defaults**

6. **Update docs and examples**
	 - Compose quickstart
	 - Helm runbook
	 - architecture diagrams
	 - external API documentation

---

## 14. Risks and mitigations

## Risk: profile picture API churn

Mitigation:

- ship compatibility layer first
- preserve old avatar reads during a transition window

## Risk: Redis Sentinel support complexity in app code

Mitigation:

- isolate Redis client initialization behind a small internal package
- keep standalone path intact for Docker and tests

## Risk: Helm chart becomes too complex

Mitigation:

- keep provider toggles coarse-grained
- keep application contracts stable
- avoid over-modeling every vendor-specific knob

## Risk: EMQX rollout changes WebSocket behavior

Mitigation:

- preserve `/ws/hdp`
- smoke test MQTT-over-WebSocket with frontend before retiring Mosquitto

---

## 15. Additional HA recommendations for power users

These are not required for the first migration wave, but they are good next steps.

1. **Use pod disruption budgets** for core stateless services.
2. **Add topology spread constraints** so frontend/gateway/auth/user pods do not co-locate unnecessarily.
3. **Add network policies** around Redis, Postgres, MinIO, and EMQX.
4. **Support external secrets** patterns so credentials are not managed only as plain Helm values.
5. **Add backup/restore runbooks** for object storage and CloudNativePG.
6. **Add liveness/readiness/startup probes** tuned per service.
7. **Prefer multiple replicas for stateless services** in Helm defaults where session behavior allows it.
8. **Add anti-affinity** for critical public-edge services like frontend and api-gateway.
9. **Document managed-provider compatibility** for users who want AWS S3, managed Redis, or managed Postgres.
10. **Consider Valkey compatibility** later if you want a cleaner long-term Redis ecosystem option, but do not block the current Redis + Sentinel plan on that decision.

---

## 16. Recommended end state

### Compose

- simple stack
- single-node EMQX
- single-node MinIO
- standalone Redis
- standalone Postgres

### Helm

- EMQX default
- MinIO-backed S3 storage default
- Redis Sentinel default
- CloudNativePG default
- app-level config abstractions
- bundled dependency mode and external dependency mode

That gives Homenavi a clean split:

- **easy local deployment by default in Docker**
- **resilient-by-default deployment in Kubernetes**
- **enough flexibility for advanced operators to bring their own platform pieces**

---

## 17. Concrete next steps

1. Add structured config values for `storage`, `redis`, `postgres`, and `mqtt`.
2. Refactor `profile-picture-service` to support S3-compatible storage.
3. Update auth/profile picture flows to use pre-signed URLs.
4. Replace Mosquitto references in Compose, Helm, and docs with EMQX-oriented config.
5. Add bundled/external provider toggles to the Helm chart.
6. Introduce CloudNativePG and Redis Sentinel as Helm defaults.
7. Update docs and architecture diagrams to reflect the new dependency model.

---

## 18. Milestones and task breakdown

The implementation should follow the existing manual-DI approach from [doc/go_service_structure_conventions.md](doc/go_service_structure_conventions.md), especially in Go services:

- configuration loaded in `internal/app/config.go`
- dependency wiring kept in `internal/app`
- narrow interfaces at service or infra boundaries
- no hidden globals or service locators

### Milestone 1: configuration and dependency seams

Goal: introduce deployment-neutral configuration seams without breaking current installs.

- [x] Add a first structured Redis config seam in `auth-service` with support for `standalone` and `sentinel` modes while preserving `REDIS_ADDR` compatibility.
- [x] Add shared config conventions for:
	- [x] `redis.mode`
	- [x] `postgres.host`
	- [x] `mqtt.url`
	- [x] `storage.type`
- [x] Update Helm values to expose the new abstractions in a consistent shape.
- [x] Update Compose env examples to keep single-node defaults explicit.
- [x] Add migration notes for old env var compatibility.

### Milestone 2: profile pictures to object storage

Goal: remove shared filesystem coupling from avatar storage.

- [x] Refactor `profile-picture-service` to depend on a storage backend abstraction.
- [x] Add local filesystem backend for compatibility.
- [x] Add S3-compatible backend for MinIO/external S3.
- [x] Add pre-signed upload flow.
- [x] Add pre-signed read/access flow.
- [x] Update auth/profile flows to store object keys or canonical asset references instead of `/uploads/...` paths.
- [x] Remove Nginx shared upload volume from the primary path.

### Milestone 3: MQTT migration to EMQX

Goal: replace Mosquitto defaults without changing the Homenavi application contract.

- [x] Introduce generic broker configuration in Compose and Helm.
- [x] Replace bundled Mosquitto deployment with EMQX in Compose.
- [x] Replace bundled Mosquitto deployment with EMQX in Helm.
- [x] Keep `/ws/hdp` externally stable while changing the upstream broker.
- [x] Update bridge and runbook guidance for EMQX equivalents.

### Milestone 4: Kubernetes HA stateful defaults

Goal: make Helm installs HA-oriented by default.

- [x] Introduce Redis Sentinel deployment mode in Helm.
- [x] Introduce CloudNativePG deployment mode in Helm.
- [x] Keep external dependency mode available for both.
- [x] Add secret/reference patterns for external services.
- [x] Add readiness, backup, and recovery guidance.

### Milestone 5: operator and power-user flexibility

Goal: let advanced users bring their own platform pieces cleanly.

- [x] Add bundled vs external provider toggles for MQTT, storage, Redis, and Postgres.
- [x] Support existing secret references instead of forcing inline Helm values.
- [x] Document bring-your-own-chart examples.
- [x] Document recommended HA options such as PDBs, topology spread, anti-affinity, and network policies.
