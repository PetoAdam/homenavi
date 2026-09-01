# Deployment Verification and User-Flow Test Plan

Status: In progress

## Implemented Foundation

- Docker Compose defaults to successfully released `latest` images; setting `HN_VERSION` still pins every Homenavi image to a specific release.
- The reusable release workflow publishes every immutable release tag. Only stable semantic version tags (`v<major>.<minor>.<patch>`) also publish `latest`; the workflow inspects every published GHCR manifest so a missing tag fails the release.
- `.env.ci`, `docker-compose.ci.yml`, and `compose_smoke.yaml` now create an isolated, branch-built Compose stack and gate on public gateway health, strict authentication, and a unique non-retained MQTT state event persisted through history-service and queried through the gateway.
- The CI override keeps hardware-only `zigbee2mqtt` out of the smoke deployment and replaces the developer PostgreSQL bind mount with a project-scoped named volume. Authentication is now a strict smoke gate; authorization, integration, command-lifecycle, and automation flows remain for the next slices. Redis remains standalone in Compose by design.
- The Minikube HA workflow now installs CNPG before Helm, validates live three-replica workloads, Redis Sentinel master discovery, complete Sentinel service configuration, and the four shared MQTT groups. It also rejects persistent-volume mounts on stateless core replicas and force-replaces one replica from each shared-MQTT tier before requiring recovery to `3/3`; the workflow always removes its isolated namespace.
- Integration-proxy is explicitly excluded from the stateless pod assertion until its mutable `installed.yaml` configuration is migrated from the `integration-proxy-config` PVC to PostgreSQL or another Kubernetes-native durable control plane. Its operation/update state is already PostgreSQL-backed, but its installation registry is not yet.

## Purpose

Homenavi needs two complementary deployment checks:

- Docker Compose verifies the complete single-node product path using PostgreSQL, EMQX, and standalone Redis.
- Minikube verifies Kubernetes-specific high-availability behavior: three replicas, Redis Sentinel, CloudNativePG, and EMQX shared subscriptions.

Neither check replaces service unit tests. Both run only against images built from the checked-out commit and use isolated data and deployment names.

## Pipeline Layout

| Check | Trigger | Runtime | Required assertions |
| --- | --- | --- | --- |
| Static deployment validation | Every relevant pull request | Docker CLI and Helm | `docker compose config --quiet`, Helm lint/template, schema validation |
| Compose smoke | Every relevant pull request | Docker Compose | Stack starts from source, edge health, API user flows, MQTT-to-history path |
| Minikube HA smoke | Every relevant pull request that changes core HA paths | Minikube | Three replicas, CNPG healthy, Sentinel master resolution, shared-group configuration |
| Browser critical-path smoke | Every pull request touching frontend, edge routing, or auth | Compose | Login and realtime UI behavior in Chromium |
| Extended integration regression | Nightly and before release | Compose, then Minikube | Full API flow matrix and selected HA disruption tests |

The Compose path filters cover Compose/environment files, Dockerfiles, shared libraries, frontend and edge configuration, bootstrap scripts, core services, and smoke tests. The Minikube path filters cover Helm, shared MQTT/Redis packages, and HA core services. Avoid separate image-build and validation pipelines that test different commits: each verification job builds its own local images with `HN_VERSION=ci-${GITHUB_SHA}`.

## Compose Verification

### CI deployment contract

Add these tracked test assets:

- `docker-compose.ci.yml`: a Compose override, never a replacement for `docker-compose.yml`.
- `.env.ci`: non-secret CI defaults with dedicated high host ports and deterministic test-only settings.
- `test/smoke/`: black-box pytest suite and helper utilities.
- `.github/workflows/compose_smoke.yaml`: Compose job.

The CI workflow must use a unique project name, for example `homenavi-ci-${{ github.run_id }}`, plus `--renew-anon-volumes`. It must not interact with a developer's local Compose project.

Workflow outline:

```sh
docker compose --env-file .env.ci -p "$PROJECT" -f docker-compose.yml -f docker-compose.ci.yml config --quiet
docker compose --env-file .env.ci -p "$PROJECT" -f docker-compose.yml -f docker-compose.ci.yml build
docker compose --env-file .env.ci -p "$PROJECT" -f docker-compose.yml -f docker-compose.ci.yml up -d --wait --wait-timeout 300
pytest -q test/smoke api-gateway/test
docker compose --env-file .env.ci -p "$PROJECT" -f docker-compose.yml -f docker-compose.ci.yml down --volumes --remove-orphans
```

The cleanup step runs with `always()`. On failure, collect `docker compose ps`, `docker compose logs --no-color`, and inspect the PostgreSQL, Redis, and EMQX containers before cleanup.

### Readiness checks

Compose currently health-checks only selected infrastructure services. The CI override should add health checks for the public edge and required core dependencies, without modifying production behavior. Readiness succeeds only after polling these gateway paths through the same API Gateway route configuration used by clients:

- `/health`
- `/api/gateway/health`
- `/api/auth/health`
- `/api/users/health`
- `/api/hdp/health`
- `/api/ers/health`
- `/api/history/health`
- `/api/automation/health`
- `/integrations/healthz`

The test runner should use a bounded retry helper and fail with the last response body. A started container, or a `docker compose up` exit code, is not sufficient proof of readiness.

Compose must remain `REDIS_MODE=standalone`. That is the supported simple-deployment topology, not an HA downgrade. Its smoke test verifies application behavior against standalone Redis; Sentinel is asserted by the Minikube job below.

## Minikube HA Verification

Retain `.github/workflows/helm_minikube_ha_smoke.yaml`, but make its contract explicit and deterministic:

1. Start Minikube with pinned CPU, memory, Kubernetes version, and Docker driver settings.
2. Install a pinned CloudNativePG operator release and wait for the controller and `clusters.postgresql.cnpg.io` CRD before Helm install.
3. Lint and render `values-ci-ha-smoke.yaml`; assert that the rendered workloads include `REDIS_MODE=sentinel`, master name, and all Sentinel addresses.
4. Install into an isolated namespace using a unique Helm release name.
5. Wait for EMQX, Redis StatefulSet, CNPG `Cluster`, and each selected three-replica Deployment.
6. Query Redis Sentinel from a Redis pod using `SENTINEL get-master-addr-by-name homenavi-redis`; require a result and three monitored Sentinels.
7. Read each live Deployment spec and require the configured groups:
   - `DEVICE_HUB_MQTT_SHARED_GROUP=device-hub-ingest`
   - `ENTITY_REGISTRY_MQTT_SHARED_GROUP=entity-registry-autoimport`
   - `HISTORY_SERVICE_MQTT_SHARED_GROUP=history-ingest`
   - `AUTOMATION_SERVICE_MQTT_SHARED_GROUP=automation-events`
8. Collect `get all`, events, and logs on failure, then uninstall the release and namespace with `always()`.

The current constrained profile is the PR gate because a GitHub-hosted runner cannot reliably schedule the full UI and adapter stack alongside nine triplicated services. Run the full `values-minikube-ha.yaml` profile nightly or before release on a self-hosted runner with documented capacity. The successful local Minikube full-profile deployment is the baseline for that job.

## API Flow Smoke Tests

Move deployment-facing tests into `test/smoke` and make them strict. Existing tests that skip when the stack, verification settings, or test account differ are useful for local exploration but are not CI gates.

Each CI run creates a unique test user and uses test-only email/verification settings. It must clean up its data or run against a discarded database volume. Required flows are:

| Flow | Required proof |
| --- | --- |
| Public edge | Frontend is served; gateway and all required service health routes return success |
| Authentication | Sign up, login, fetch `/me`, refresh token, logout, and verify refresh-token rejection |
| Authorization | Resident cannot invoke an admin-only mutation; seeded admin can perform the allowed management action |
| Device state | Publish a unique HDP state event through EMQX, observe its device representation through the gateway, and confirm ERS registration/binding |
| History | The same event produces a queryable history record for its exact device ID and payload |
| Command lifecycle | Submit a command, publish the matching device acknowledgement, and assert its terminal status once only |
| Automation | Create or seed a minimal event-triggered automation, publish the event, and assert one completed run plus its persisted run event |
| Integration health | Verify integration-proxy health and list the built-in/test registry; defer external marketplace installation to a separately sandboxed job |

Tests must use a unique run prefix in emails, device IDs, MQTT client IDs, automation names, and correlation IDs. They must poll only eventual-consistency boundaries (MQTT-to-database processing) and use short fixed deadlines. Do not use unconditional sleeps or accept an empty history response as success.

Add a small Python MQTT fixture using the project-supported broker protocol. It publishes retained and non-retained HDP payloads, waits for a predicate through public APIs, and captures broker logs when a predicate expires. This makes the device, history, and automation tests portable between Compose and a port-forwarded Minikube edge.

## Browser Smoke Tests

Add Playwright under `frontend/` after the API smoke contract is stable. Run it against the Compose public nginx endpoint, with Chromium only for pull requests and the cross-browser matrix nightly. The initial required browser scenarios are:

1. Login and session restoration after reload.
2. Dashboard loads an authenticated user’s widgets and navigation remains usable.
3. A fixture-published MQTT device update reaches the visible device/dashboard view through `/ws/hdp`.
4. A user creates or edits a minimal automation and observes its completed run state.

Use API setup/cleanup for fixtures. Browser tests should validate user-visible outcomes, not duplicate every API status assertion. Capture trace, screenshot, video, browser console, and network logs only on failure.

## Delivery Order

1. Add `.env.ci`, `docker-compose.ci.yml`, and the Compose lifecycle workflow with health-only checks.
2. Convert authentication tests into strict CI smoke tests with deterministic test-user setup.
3. Add MQTT fixture plus device, ERS, history, command, and automation API flows.
4. Harden the existing Minikube workflow with CNPG installation, live Sentinel queries, live environment assertions, and guaranteed cleanup.
5. Add the nightly full-HA self-hosted job and selected disruption tests: restart one core replica and one Redis replica, then repeat the relevant flow.
6. Add Playwright critical paths and make them required for frontend, gateway, and auth changes.

## Exit Criteria

The verification system is complete when a pull request cannot merge with a broken Compose product path, a missing Sentinel/shared-subscription Helm setting, or a broken core user flow; nightly runs additionally prove the full three-replica profile and controlled recovery from a single replica restart.