# MQTT, Valkey, and Licensing Migration Plan

## Goals

- Make Mosquitto the default broker for single-node Docker Compose and Helm deployments.
- Preserve EMQX as an explicitly selected compatibility option, documented as a single-node choice under its current license terms.
- Add VerneMQ as the recommended clustered broker for HA Helm deployments.
- Replace Redis server images with Valkey while retaining Redis-protocol client compatibility.
- Establish repeatable third-party attribution and license scanning for source and image builds.

## Verified Licensing Inputs

- Mosquitto is dual-licensed under EPL-2.0 or EDL-1.0 (BSD-3-Clause).
- Current EMQX releases are under BSL 1.1. The additional production-use grant permits a single node; clustered or hosted/embedded production use requires separate review and potentially a commercial license.
- Valkey is BSD-3-Clause. This is permissive for commercial and SaaS use, subject to retaining required notices; this plan is not legal advice.
- `ghcr.io/micw/vernemq-docker` is a community-built Apache-2.0 image project. Its README states that official VerneMQ Docker builds require commercial payment from version 1.10 onward, while the custom build is Apache-2.0. Pin its digest and verify provenance, SBOM, vulnerabilities, and upstream version support before adopting it.

## Scope And Decisions

### MQTT Broker Profiles

| Profile | Broker | Shared subscriptions | Intended use |
| --- | --- | --- | --- |
| Default | Mosquitto with MQTT 5 | Enabled | Docker Compose and single-node Helm |
| Compatibility | EMQX | Enabled | Explicit opt-in single-node deployments only |
| HA | VerneMQ community image | Enabled | Helm HA deployments with broker clustering |

All application services remain broker-protocol agnostic. Keep `$share/{group}/{topic}` as the shared-subscription syntax and preserve the existing `MQTT_SHARED_GROUP` environment variables. Broker selection belongs in deployment configuration, not service business logic.

## Implementation Phases

### 1. Establish The Broker Configuration Contract

1. Audit `shared/mqttx` broker-kind validation and remove the current generic/Mosquitto shared-subscription rejection.
2. Introduce explicit broker kinds: `mosquitto`, `emqx`, and `vernemq`; reject unsupported values and keep shared/exclusive subscription mode validation centralized.
3. Add a broker capability table in code and tests. Mosquitto, EMQX, and VerneMQ must all allow MQTT 5 shared subscriptions; the app must not rely on vendor-only topic syntax.
4. Preserve environment compatibility: `MQTT_BROKER_URL`, `MQTT_SHARED_GROUP`, and `MQTT_WEBSOCKET_UPSTREAM_URL` continue to work. Add `MQTT_BROKER_KIND` only where selection is required.
5. Add integration tests that connect a publisher plus multiple consumers to each supported broker image and assert load-balanced delivery through `$share` topics.

Primary surfaces: `shared/mqttx/`, service configuration tests, `.env.example`, and integration test Compose files.

### 2. Make Mosquitto The Default Single-Node Broker

1. Add a version-pinned Mosquitto MQTT 5 service and configuration under `mosquitto/`.
2. Switch `docker-compose.yml`, `.env.example`, local scripts, and Helm default broker values from EMQX to Mosquitto.
3. Preserve the existing `emqx/` assets behind `broker.provider: emqx`; do not delete them in this migration.
4. Update health checks, WebSocket listener configuration, bridge guidance, and port documentation for Mosquitto.
5. Add a Compose smoke profile that proves all MQTT consumers start with shared subscriptions enabled against Mosquitto.

Acceptance criteria: a fresh Compose deployment uses Mosquitto, publishes HDP state, and distributes shared subscriptions across replica consumers.

### 3. Add VerneMQ For HA Broker Clustering

1. Add `broker.provider: vernemq` Helm values, templates, ConfigMaps, Services, StatefulSet, headless Service, PodDisruptionBudget, anti-affinity, and persistent storage.
2. Use a digest-pinned `ghcr.io/micw/vernemq-docker` image, never the official VerneMQ binary image. Record the image source, digest, upstream release, and Apache-2.0 attribution in `NOTICE.md`.
3. Configure cluster discovery through Kubernetes DNS/StatefulSet peer names and expose MQTT 1883 plus WebSocket only when enabled.
4. Define a supported HA topology: at least three replicas, quorum-safe disruption budget, persistent node identity, and documented scale-up/down and recovery procedures.
5. Keep `broker.provider: emqx` available but block or warn when its replica count is greater than one. Document that EMQX clustering is not supported under the default licensing posture.
6. Make HA overlays (`values-minikube-ha.yaml`, CI HA smoke values, production HA examples) select VerneMQ. Add a three-broker smoke test that verifies surviving shared-subscription delivery after one broker pod is removed.

Acceptance criteria: Helm HA deploys a three-node VerneMQ cluster, application replicas share subscriptions, and a single broker failure does not halt command/state traffic.

### 4. Replace Redis Server Images With Valkey

1. Replace Compose `redis` server containers with a pinned official Valkey image.
2. Replace Helm Redis/Sentinel server images with compatible Valkey and Valkey Sentinel images. Retain existing Kubernetes Service names and `REDIS_*` environment variables in the first migration to avoid application DNS/config churn.
3. Keep `github.com/redis/go-redis/v9`: it speaks the compatible protocol and does not require an immediate client-library rename. Record this compatibility decision in the architecture and deployment docs.
4. Retain the existing Sentinel topology, PDB, persistent volumes, and service endpoints, but update labels, comments, image references, and operator-facing names from Redis to Valkey where safe.
5. Define an explicit data migration/rollback procedure for existing persistent deployments: backup, create a compatible Valkey target, restore/replicate, verify sessions/cache behavior, then cut over. Do not silently attach a Valkey StatefulSet to existing Redis data without compatibility testing.
6. Update HA smoke tests to validate Valkey Sentinel failover and client reconnection.

Acceptance criteria: Compose and Helm deploy Valkey, all Go services connect using their existing configuration, and Sentinel failover remains green.

### 5. Add Attribution And License Inventory

1. Create root `NOTICE.md` in `homenavi` with the project attribution policy and generated third-party notices for distributed dependencies, images, fonts, icons, and bundled frontend assets.
2. Preserve existing icon attribution and reference it from `NOTICE.md`; do not suppress upstream attribution UI unless its license permits it.
3. Add a reproducible script that inventories every Go module in the workspace with `go-licenses`, including all service modules and integrations where their images are built.
4. Add `vite-plugin-license` to Vite applications and emit a machine-readable inventory plus a human-readable notice artifact during production builds.
5. For the Next.js website, use an equivalent Node dependency license inventory rather than incorrectly applying a Vite plugin. Keep its notice artifact separate or link to the core notice policy.
6. Add base-image and broker-image entries manually or through SBOM tooling, because package-level Go/npm scanners do not cover container base images.
7. Review scanner output before committing generated notices. Mark unknown, missing, copyleft, and custom licenses as review-required rather than automatically accepting them.

### 6. Gate Image Builds On License Scans

1. Add a reusable Go license-scan workflow/action invoked before each Go service image release. Cache tool installation and fail on unreviewed/forbidden licenses according to an allowlist policy.
2. Add Vite license generation and verification to the frontend image build workflow; copy the generated notice artifact into the runtime image or publish it alongside the image artifact.
3. Add the equivalent website/Next.js inventory check to its CI and release workflow.
4. Generate SPDX or CycloneDX SBOMs for every released image and attach them as workflow artifacts or OCI attestations.
5. Add a release check that verifies `NOTICE.md` and generated inventories are current when dependency manifests or lockfiles change.

## Documentation And Website Updates

Update the following once implementation begins:

- Core README, `.env.example`, Compose comments, deployment guide, MQTT architecture/topology docs, Helm README, HA operations guide, and migration helper documentation.
- Website deployment wizard, deployment profiles, architecture diagrams, feature/technology lists, and broker/HA comparison copy.
- Replace Redis/EMQX default references with Mosquitto/Valkey, while clearly showing EMQX as opt-in and VerneMQ as the HA recommendation.
- Document image provenance and the VerneMQ community-image constraint in both deployment docs and `NOTICE.md`.

## Validation Matrix

1. Go unit tests for broker capability validation and Redis/Valkey configuration parsing.
2. Compose smoke tests for Mosquitto shared subscriptions and Valkey connectivity.
3. Helm lint/template tests for every broker provider and Valkey mode.
4. HA smoke tests for three-node VerneMQ and Valkey Sentinel failover.
5. End-to-end tests for MQTT command delivery, device-hub state fan-out, and MCP/session cache behavior.
6. Website typecheck, unit tests, build, and browser validation for updated deployment/architecture pages.
7. License scan, notice-generation diff check, SBOM generation, and image vulnerability/provenance checks.

## Rollout And Rollback

1. Ship broker-provider and Valkey support behind explicit values first; do not flip defaults in the same release.
2. Validate Mosquitto and Valkey in CI, demo, and a staging namespace.
3. Migrate HA to VerneMQ with a documented maintenance window and MQTT client reconnect monitoring.
4. Flip default Compose/Helm profiles only after successful staged validation.
5. Preserve EMQX/Redis compatibility values for one documented deprecation window. Roll back by restoring the prior provider/image values, not by deleting persistent data.