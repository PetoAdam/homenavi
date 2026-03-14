# Homenavi Deployment Modes (Compose or Helm) Implementation Plan

Status: Active implementation (MVP-1 focus: Minikube Helm local)  
Date: 2026-03-01  
Owners: Core Platform + Integration Proxy + Marketplace

---

## 1) Executive Summary

Homenavi should support **two equal deployment modes**:

- **Docker Compose** for hobbyist and simpler environments
- **Helm/Kubernetes** for homelab and advanced environments

Neither mode is the primary installation path. Product messaging, docs, and APIs must treat both as first-class.

To make this maintainable and high quality, deployment behavior must be abstracted from HTTP handlers in `integration-proxy` into runtime adapters (`compose`, `helm`) behind a stable application service boundary.

This document defines architecture, contracts, migration, rollout, and acceptance criteria.

Current execution priority:

1. Get the full Homenavi core running on local Minikube via Helm.
2. Get a local Homenavi Marketplace running on the same Minikube cluster via Helm.
3. After MVP runtime is stable, expand CI/validation/verification hardening.

---

## 2) Goals and Non-Goals

### Goals

1. Provide full Homenavi deployment via Helm chart in addition to existing Docker Compose.
2. Evolve `integration-proxy` to manage integration lifecycle for the selected environment runtime:
   - install
   - update
   - restart
   - uninstall
3. Keep Compose and Helm equal in product stance and support level.
4. Standardize integration metadata on `deployment_artifacts` (breaking changes allowed during alpha).
5. Allow optional Kubernetes fallback from Compose artifacts using Kompose.
6. Improve code quality with clear architecture layers, testability, and explicit runtime contracts.

### Non-Goals (for initial release)

1. Full GitOps controller/operator for integrations.
2. Multi-cluster orchestration.
3. Dynamic Helm chart authoring UI in dashboard.
4. Full parity for every edge case in Kompose output.

---

## 3) Current State (Baseline)

Based on current code and docs:

- `integration-proxy` supports environment runtime mode (`compose|helm|gitops|auto`) with runtime-aware lifecycle handlers.
- Marketplace and release payloads support canonical `deployment_artifacts` metadata.
- Helm scaffold for Homenavi core exists at `helm/homenavi`.
- A dedicated Helm chart for local `homenavi-marketplace` deployment is not in place yet.

Implication: the remaining MVP gap is end-to-end local Helm deployment completeness (all core services + marketplace) and runbook-driven operability on Minikube.

---

## 4) Target Architecture

### 4.1 Architectural Style

Apply layered design in `integration-proxy`:

1. **Transport layer**: HTTP handlers (validation, auth, mapping req/res)
2. **Application layer**: lifecycle use-cases (`InstallIntegration`, `UpdateIntegration`, etc.)
3. **Domain contracts**: runtime interfaces, operation context, operation result
4. **Infrastructure adapters**:
   - Compose runtime adapter
   - Helm runtime adapter
  - Artifact resolution adapter

### 4.2 Runtime Abstraction

Introduce core interface (conceptual):

```text
DeploymentRuntime
  - Name() string
  - Install(ctx, spec)
  - Update(ctx, spec)
  - Restart(ctx, spec)
  - Uninstall(ctx, spec)
  - Health(ctx, spec)
```

Where `spec` includes integration ID, version, resolved environment runtime, deployment artifacts, env/secret refs, and operation options.

### 4.3 Runtime Resolution

Runtime chosen by this precedence:

1. Explicit environment runtime config (for example `INTEGRATIONS_RUNTIME_MODE=compose|helm|gitops`)
2. Auto-detected runtime from host capabilities (when runtime mode is `auto` or unset)
3. Fail startup with actionable error if environment is ambiguous/unsupported

No global product preference between Compose and Helm; only explicit environment policy.

Auto-detection (default behavior):

- If Kubernetes in-cluster context is detected (for example service account + `KUBERNETES_SERVICE_HOST`) -> default to `helm`
- Else if Docker socket / Compose toolchain is available -> default to `compose`
- Else -> fail and require explicit environment runtime configuration

Important guardrail: auto-detection runs only during environment bootstrap/startup. Runtime does not vary per integration request.

### 4.4 One Runtime Per Environment Policy

Homenavi environments must run a single integration lifecycle runtime:

- `compose` environment: all integrations are managed with Compose.
- `helm` environment: all integrations are managed with Kubernetes/Helm artifacts.

Mixed-runtime installs in one environment are out of scope and should be rejected by validation.

Rationale:

- one control plane for lifecycle actions
- consistent networking/secrets model
- predictable upgrades and incident handling

### 4.5 GitOps Acknowledgement (Kubernetes)

For Kubernetes users running ArgoCD/Flux or equivalent GitOps workflows:

- manage Homenavi and integrations from the GitOps repository
- do not use in-app marketplace install/update actions as the source of truth

This model works as long as integration deployment artifacts are available and the GitOps repo declares the desired integration releases/configuration.

---

## 5) Deployment Artifact Strategy

### 5.1 Required Metadata Evolution

Current metadata is Compose-oriented. Standardize on runtime-neutral `deployment_artifacts`.

Canonical model:

```json
{
  "deployment_artifacts": {
    "compose": {
      "file": "compose/docker-compose.integration.yml"
    },
    "k8s_generated": {
      "kind": "helm_chart",
      "source": "compose_to_k8s_pipeline",
      "chart_ref": "oci://ghcr.io/.../homenavi-example-integration-generated",
      "version": "0.1.0"
    },
    "helm": {
      "chart_ref": "oci://ghcr.io/.../homenavi-example-integration",
      "version": "0.1.0",
      "values_schema_url": "https://.../values.schema.json",
      "provenance": "https://.../attestations/0.1.0"
    }
  }
}
```

Artifact precedence for Kubernetes installs:

1. Native `deployment_artifacts.helm`
2. Pipeline-generated `deployment_artifacts.k8s_generated`
3. Fail (no Kubernetes-capable artifact)

### 5.2 Integration Config Persistence (installed.yaml)

Persist integration artifact references and operation metadata per integration (runtime is environment-level):

```yaml
integrations:
  - id: homenavi-example-integration
    upstream: http://homenavi-example-integration:8099
    version: 0.1.0
    auto_update: true
    deployment:
      compose:
        file: integrations/compose/homenavi-example-integration.yml
      helm:
        release_name: homenavi-int-homenavi-example-integration
        namespace: homenavi-integrations
        chart_ref: oci://ghcr.io/.../homenavi-example-integration
        values_file: integrations/helm/homenavi-example-integration.values.yaml
      fallback:
        used_kompose: false
      provenance:
        artifact_source: helm_native # helm_native | k8s_generated
        artifact_version: 0.1.0
```

Rule: update operations must use the resolved environment runtime; per-integration runtime overrides are not allowed.

---

## 6) Helm for Homenavi Core

### 6.1 Chart Scope

Create `helm/homenavi` in main repo with:

- API and core backend services
- frontend
- integration-proxy
- service dependencies currently in compose (as applicable)
- ingress/service configs
- PVC options for stateful components

### 6.2 Design Principles

1. Keep parity with existing Compose defaults.
2. Avoid hidden behavior differences between Compose and Helm.
3. Expose values for major tuning knobs only; keep sane defaults.
4. Support Kubernetes-native secret injection for sensitive env vars.

### 6.3 CI for Helm

Add checks:

- `helm lint`
- `helm template`
- optional `kubeconform`/schema validation
- smoke deploy in `kind` for critical path

---

## 7) Integration Proxy Refactor Plan

### 7.1 Internal Packages

Suggested package split under `integration-proxy/internal`:

- `application/lifecycle` (use-cases)
- `domain/deployment` (interfaces + models)
- `infra/runtime/compose`
- `infra/runtime/helm`
- `infra/artifacts` (artifact resolution and metadata normalization)

### 7.2 Handler Responsibilities After Refactor

Handlers should only:

1. parse+validate request
2. auth check
3. call lifecycle service
4. map service result/error to response

No direct shell/runtime command orchestration in handlers.

### 7.3 Lifecycle Semantics

#### Install

1. Validate metadata/runtime availability and one-runtime-per-environment policy
2. Resolve deployment artifact matching environment runtime
3. Prepare secrets/config references
4. Execute runtime install
5. Persist config transactionally
6. Reload or refresh proxy state

#### Update

1. Resolve latest artifact from marketplace
2. Use environment runtime for update/upgrade (compose pull+up OR helm upgrade)
3. Persist installed version
4. Refresh manifest/status

Update guardrail:

- Never auto-switch runtime during update (e.g., `compose` -> `helm` or `helm` -> `compose`).
- Runtime change requires explicit migration endpoint/workflow.

#### Uninstall

1. Runtime uninstall
2. Remove config state
3. Cleanup optional generated files

#### Restart

1. Runtime-specific restart semantics
2. status update + manifest refresh

---

## 8) Kompose Fallback Strategy

### 8.1 Positioning

Kompose remains **compatibility-only** and should run in integration release pipelines, not in runtime install paths.

Goal: ship versioned Kubernetes-capable artifacts ahead of time, so install/update only consumes published artifacts.

### 8.2 Pipeline Transformation Model

When integration release pipeline runs:

1. Validate compose artifact.
2. Run Kompose conversion in CI.
3. Apply normalization patches (labels, probes, resource defaults, namespace/release naming conventions).
4. Package generated Kubernetes artifact (prefer generated Helm chart or vetted manifests bundle).
5. Publish artifact with same integration version tag.
6. Publish metadata pointers in marketplace as `k8s_generated` artifact.

Install/update path then only pulls versioned artifacts; no live Compose->Kubernetes conversion.

### 8.3 Runtime Decision Logic

When environment runtime is `helm`:

1. If native Helm artifact exists -> use Helm directly.
2. Else if pipeline-generated Kubernetes artifact exists -> use generated artifact.
3. Else -> fail with clear remediation message.

### 8.4 Feature Flags

Proposed env flags:

- `INTEGRATIONS_RUNTIME_MODE=auto|compose|helm|gitops`
- `INTEGRATIONS_K8S_GENERATED_ARTIFACTS_ENABLED=true`

### 8.5 Known Limitations to Document

Generated artifacts from Kompose may not preserve:

- advanced networking assumptions
- healthchecks/probes parity
- storage semantics
- security context expectations

Mark generated-artifact installs as `best_effort` in status/API unless they pass stricter certification gates.

### 8.6 Certification Gates for Generated Artifacts

Before publishing generated artifact:

1. Render/lint validation must pass.
2. Smoke deploy in ephemeral cluster must pass.
3. Basic readiness/health endpoint checks must pass.
4. Security policy checks (image/signature/provenance where applicable) must pass.

---

## 9) API and Contract Changes

### 9.1 Install Request

Extend payload:

```json
{
  "id": "homenavi-example-integration",
  "upstream": "http://homenavi-example-integration:8099",
  "version": "0.1.0",
  "auto_update": true
}
```

Breaking change policy (alpha):

- Runtime is environment-level and not accepted in install/update requests.
- `compose_file` request field is removed in favor of marketplace `deployment_artifacts` and runtime-specific resolution.
- Any request attempting per-integration runtime override is rejected.

### 9.2 Status Response

Include runtime and fallback info:

```json
{
  "id": "homenavi-example-integration",
  "stage": "ready",
  "runtime": "helm",
  "fallback": { "used_kompose": false },
  "updated_at": "..."
}
```

### 9.3 Marketplace API

Expose normalized deployment artifact fields so clients and proxy can choose runtime without custom heuristics.

### 9.4 Runtime Migration API (explicit)

Add explicit environment migration operation (example: `POST /environment/migrate-runtime`) to move the environment from `compose` to `helm` (or reverse), with preflight checks and rollback semantics.

This avoids hidden behavior changes in ordinary update flows.

### 9.5 GitOps Runtime Mode (Kubernetes)

Add an optional GitOps mode for Kubernetes environments:

- in-app marketplace install/update operations are disabled or read-only
- integration state is observed from configured manifests/releases
- API responses should indicate `management_mode: gitops`

This keeps operational ownership aligned with ArgoCD/Flux reconciliation.

---

## 10) Repository-by-Repository Work Breakdown

### 10.1 `homenavi`

1. Add `helm/homenavi` chart and values files.
2. Add docs for deployment mode options with equal positioning.
3. Add CI checks for helm chart quality.
4. Refactor `integration-proxy` runtime orchestration.
5. Add/adjust env var docs for runtime selection.
6. Enforce one-runtime-per-environment policy in install/update validation.
7. Add optional GitOps mode flags and read-only marketplace behavior.

### 10.2 `homenavi-marketplace`

1. Extend models/store for `deployment_artifacts`.
2. Remove `compose_file` from canonical API model.
3. Return normalized deployment data in API responses.
4. Add migration for DB schema if required.

### 10.3 `homenavi-integration-template`

1. Update metadata/schema to support deployment artifacts.
2. Add optional Helm scaffold guidance.
3. Keep Compose path as valid template default.

### 10.4 Integration repos (e.g. `homenavi-lg-thinq`, `homenavi-example-integration`)

1. Initially no forced changes.
2. Optional: publish native Helm artifact for best Kubernetes support.
3. If no native Helm artifact, pipeline may publish generated Kubernetes artifact from Compose.
4. Runtime install should consume published artifacts only (no live conversion in proxy).

---

## 11) Data and Config Migration Plan

### 11.1 Existing installed integrations

- No compatibility mapping for pre-standardized deployment fields.
- Config must use runtime-specific artifact references.
- Reject startup if invalid/missing deployment metadata is found.

### 11.2 Marketplace metadata migration

1. Add required `deployment_artifacts` fields.
2. Remove legacy Compose-only fields from published contract.
3. Enforce strict schema validation in publish/verify pipelines.

### 11.3 Rollback

- Runtime adapter errors do not corrupt config state.
- Save-before-apply vs apply-before-save must be standardized with rollback actions.

---

## 12) Testing Strategy

### 12.1 Unit Tests

1. Runtime resolver and policy selection.
2. Compose adapter command generation.
3. Helm adapter command generation.
4. Kompose fallback decision logic.
5. Handler request schema enforcement rejecting per-integration runtime fields.
6. Policy validation rejecting mixed-runtime installs in one environment.

### 12.2 Integration Tests

1. Install/update/uninstall for Compose runtime.
2. Install/update/uninstall for Helm runtime (kind in CI or dedicated environment).
3. Generated Kubernetes artifact install/update scenario.
4. Verify update does not switch runtime automatically.
5. Explicit runtime migration flow test coverage.
6. Kubernetes GitOps mode with marketplace actions disabled/read-only.

### 12.3 Contract Tests

1. Marketplace response contract for deployment artifacts.
2. Integration template schema validation for deployment metadata.

---

## 13) Security and Operational Considerations

1. Compose mode with Docker socket remains high-trust; keep current warning in docs.
2. Helm mode should use scoped service accounts and namespace isolation.
3. Secrets should be injected per runtime using least privilege.
4. Audit logs should include runtime, integration ID, version, and operation result.
5. Avoid shell injection risk by structured command arg construction only.

---

## 14) Implementation Phases and Milestones

### Phase M1 — Minikube Helm MVP (current priority)

- Complete `helm/homenavi` so the full core stack runs on local Minikube.
- Add `homenavi-marketplace` Helm chart for local Minikube deployment.
- Publish and validate a single local runbook for:
  - cluster bootstrap
  - core Helm install
  - marketplace Helm install
  - smoke verification

### Phase M2 — Post-MVP Hardening

- CI checks for Helm quality and template rendering.
- Runtime integration tests (Compose and Helm paths).
- Validation/verification hardening and release readiness checks.

### Phase A — Foundations (completed/in progress)

- ADRs finalized
- Runtime domain contracts defined
- Helm chart skeleton for Homenavi core

### Phase B — Integration Proxy Runtime Refactor (1-2 sprints)

- Compose logic moved into adapter
- Lifecycle service introduced
- HTTP handlers simplified

### Phase C — Helm Runtime + Marketplace Contracts (1-2 sprints)

- Helm runtime adapter implemented
- marketplace metadata/API updated
- template schema updated

### Phase D — Kompose Compatibility Bridge (optional, 1 sprint)

- pipeline-generated Kubernetes artifact flow
- best-effort status surfaced where applicable
- docs and caveats published

### Phase E — Stabilization and GA (1 sprint)

- end-to-end tests
- docs polish
- release readiness checks

---

## 15) Definition of Done (DoD)

### MVP-1 DoD (local Helm)

MVP-1 is complete when:

1. Homenavi core runs on local Minikube via Helm.
2. Homenavi Marketplace runs on the same local Minikube via Helm.
3. Basic smoke flow works locally (core reachable, marketplace API reachable, listing endpoint responds).
4. Documentation provides a deterministic local runbook with commands and verification checks.

### Full Delivery DoD

Implementation is complete when:

1. Homenavi can be installed and run via Compose and via Helm.
2. `integration-proxy` lifecycle operations work for both runtimes.
3. New metadata format with deployment artifacts is enforced end-to-end.
4. CI validates Helm artifacts and runtime tests pass (post-MVP hardening).
5. Docs explicitly present Compose and Helm as equal supported options.
6. Mixed-runtime installs are rejected by policy.
7. GitOps mode behavior is documented and test-covered.

---

## 16) Risks and Mitigations

1. **Risk:** Kompose output unpredictability  
  **Mitigation:** CI-time generation + certification gates + best-effort label + native Helm recommendation.

2. **Risk:** Migration regressions in marketplace/install payloads  
  **Mitigation:** Strict schema validation + explicit migration tooling + contract tests.

3. **Risk:** Increased operational complexity  
   **Mitigation:** clear docs and per-environment quickstart guides.

---

## 17) Product and Documentation Requirements

1. Keep both installation guides visible in top-level docs.
2. Do not label one as preferred globally.
3. Use audience guidance only:
   - Compose: simpler local/hobby setups
   - Helm: Kubernetes/homelab advanced setups
4. Ensure admin UI messaging mirrors this neutral positioning.

---

## 18) ADRs to Create

1. `ADR-00X`: Deployment Mode Selection and One-Runtime-Per-Environment Policy
2. `ADR-00Y`: Integration Proxy Runtime Adapter Architecture
3. `ADR-00Z`: Pipeline-Based Compose-to-Kubernetes Artifact Strategy

---

## 19) Immediate Next Steps (Actionable)

1. Create the three ADRs listed above.
2. Open epic tickets in each repo for scoped work packages.
3. Implement `integration-proxy` runtime interface + compose adapter extraction first.
4. Add `helm/homenavi` chart scaffold and CI lint/template checks.
5. Design and merge marketplace metadata schema changes with strict validation tests.

---

## 20) Appendix

### 20.1 Environment Runtime Resolution Matrix

| `INTEGRATIONS_RUNTIME_MODE` | Kubernetes In-Cluster Detected | Docker/Compose Available | Resolved Mode |
|---|---:|---:|---|
| `compose` | any | any | compose |
| `helm` | any | any | helm |
| `gitops` | any | any | gitops |
| `auto`/unset | yes | any | helm |
| `auto`/unset | no | yes | compose |
| `auto`/unset | no | no | startup error (explicit mode required) |

### 20.2 Integration Install Matrix by Environment Mode

| Environment Mode | Native Helm Artifact | Generated K8s Artifact | Compose Artifact | Result |
|---|---:|---:|---:|---|
| compose | n/a | n/a | yes | Install via Compose |
| compose | n/a | n/a | no | Fail (missing Compose artifact) |
| helm | yes | any | any | Install via native Helm artifact |
| helm | no | yes | any | Install via generated K8s artifact |
| helm | no | no | yes | Fail with remediation (generate/publish K8s artifact) |
| helm | no | no | no | Fail (missing artifacts) |
| gitops | any | any | any | Read-only in app; install/update managed from GitOps repo |

### 20.3 Update Runtime Stability Matrix

| Environment Mode | Requested Action | Runtime Change Requested | Result |
|---|---|---:|---|
| compose | update | no | Compose update |
| helm | update | no | Helm update |
| gitops | update | no | Reject in app; managed by GitOps reconciliation |
| compose | update | yes (to helm) | Reject; require environment migrate-runtime |
| helm | update | yes (to compose) | Reject; require environment migrate-runtime |
