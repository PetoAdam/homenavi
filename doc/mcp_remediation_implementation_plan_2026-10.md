# MCP Remediation Implementation Plan (2026-10)

> Status: proposed. This document is an implementation and testing plan only.
> It does not change deployed behavior. It follows the findings in
> [MCP Review Findings](mcp_review_findings_2026-10.md).

## Goals

1. Make consent removal terminate the removed client's MCP and delegated access immediately, without logging the user out or disconnecting other clients.
2. Make an unset `MCP_RESOURCE_URI` work for authenticated MCP domain tools, not only discovery metadata.
3. Make MCP discovery tools describe only capabilities that can actually be used.
4. Bring current operator/client documentation in line with deployed behavior.
5. Reduce `server.go` coupling only after the behavior is protected by tests.

## Non-Goals

- Do not add pairing, automation-editing, dashboard-editing, account-editing, raw WebSocket, MQTT, or event-subscription MCP capabilities.
- Do not change public OAuth endpoint paths, grant types, client registration rules, or existing scope names.
- Do not revoke the user's browser session when one connected application is removed.
- Do not silently accept blank resource configuration until the derived-resource path is proven by an authenticated tool-call test.

## Design Decisions

### Grant-specific revocation model

Use an opaque, random MCP grant ID, not a per-token deny list or the existing user-session status.

- A consent approval creates an opaque `grant_id`.
- The consent record stores `grant_id`, client ID, resource, scopes, and timestamp.
- Auth-service stores `auth_mcp_grant:<grant_id> = active` in Redis with no expiry while the consent is active.
- MCP access tokens and delegated tokens carry a shared claim such as `gnt` (`authx.ClaimMCPGrantID`).
- Revoke deletes the consent record and the `auth_mcp_grant:<grant_id>` key.
- Reapproval creates a new grant ID. Tokens bearing the removed grant ID remain invalid even if the same client is approved again.

This is compatible with the existing shared Redis session-status pattern, avoids importing auth-service internals into mcp-service or api-gateway, and enables immediate validation at each authorization boundary.

### Revocation policy

The target policy is immediate rejection of every future request after client removal:

| Boundary | Target behavior |
| --- | --- |
| Authorization code issue | Requires an active consent/grant |
| OAuth refresh | Requires the original active grant |
| `/mcp` request | Requires an active session and active grant |
| MCP-to-gateway token exchange | Requires an active session and active grant |
| Delegated gateway route | Requires an active session and active grant |

This deliberately invalidates an already-issued two-minute delegated token at the gateway. A bounded-lifetime exception is not recommended because it conflicts with the Connected Applications “Remove access” expectation and leaves an avoidable control window.

All grant-status lookup failures must fail closed as `401` or `503` according to the existing authentication error convention; they must never be interpreted as active.

### Canonical resource policy

Resolve the effective MCP resource exactly once per request:

1. Use validated `MCP_RESOURCE_URI` when configured.
2. Otherwise derive `https://<forwarded-host>/mcp` from the first `X-Forwarded-Host` and `X-Forwarded-Proto` values, falling back to the request host/TLS only for direct deployments.
3. Validate it with the existing `validMCPResource` rules.
4. Store the effective resource in `mcpPrincipal`.
5. Use `principal.resource` for every delegated token exchange.

The configured value remains the operationally preferred production setting. Derivation is a supported fallback, not a substitute for reliable proxy configuration.

## Implementation Work Packages

### 1. Add shared grant identity primitives

**Files:** `shared/authx/claims.go`, new focused shared-auth tests if this package has a test convention.

1. Add `ClaimMCPGrantID`, with a short unambiguous claim name such as `gnt`.
2. Add `MCPGrantStatusKey(grantID string)` that returns `auth_mcp_grant:<grantID>`.
3. Do not expose grant IDs in browser UI, API responses, logs, or audit fields. They are authorization state, not user-facing identifiers.
4. Use the same opaque-random ID generator already used for token IDs; do not derive an ID from subject or client ID.

**Acceptance criteria:** the key format and claim are shared constants, and no service duplicates their literals.

### 2. Make OAuth consent a revocable grant

**Files:** `auth-service/internal/auth/oauth_consent.go`, `oauth_codes.go`, `oauth_service.go`, refresh-token record helpers/tests, and OAuth HTTP handler interfaces only if return signatures need to change.

1. Extend `OAuthConsent` with `GrantID`.
2. Replace the boolean-only consent lookup internally with a method that returns the matching current consent record. Retain a thin boolean helper only where it avoids unnecessary churn.
3. On explicit approval, create a new grant ID and persist both the consent record and active grant-status key.
4. On reapproval for an existing `(subject, clientID)`, revoke the old grant status before activating the replacement. If an intermediate storage failure occurs, favor a revoked/disconnected client over preserving old access.
5. On revoke, load the consent first, delete its grant-status key, then delete the consent record. Make repeated revoke requests idempotent.
6. Extend `OAuthAuthorizationGrant`, authorization-code records, and refresh-token records with `GrantID`.
7. Require a non-empty `GrantID` when issuing an MCP access token or renewing an OAuth grant. Older records without one must be treated as invalid rather than receiving a compatibility bypass.
8. Add `gnt` to MCP access-token claims and copy it into delegated token claims.
9. In `RefreshOAuthAuthorizationGrant`, validate the consent record and require its grant ID to match the stored refresh-token grant ID.
10. In `ExchangeMCPAccessToken`, require a valid `gnt` claim and active grant-status key in addition to current issuer, audience, token type, session, role, and scope checks.

**Race and recovery rules:**

- Never issue a code, refresh token, MCP token, or delegated token from a consent with no active grant-status key.
- A delayed request using a revoked grant ID must fail after the delete operation completes.
- A fresh consent creates a distinct grant ID, so it cannot reactivate old bearer tokens.
- Keep the current refresh-token replay behavior unchanged: replay continues to revoke the browser session/family as it does today.

**Acceptance criteria:** a removed client cannot obtain or exchange a token, while a second client for the same user and the user’s web session remain usable.

### 3. Enforce grant state at MCP and gateway boundaries

**Files:** `mcp-service/internal/mcpserver/server.go`, `mcp-service/internal/app/app.go`, `api-gateway/internal/middleware/auth.go`, `api-gateway/internal/http/router.go`, related middleware tests.

1. Extend `mcpPrincipal` with `grantID` and `resource`.
2. Extend `mcpserver.Config` with `GrantValidator(ctx, grantID) (bool, error)`, paralleling `SessionValidator`.
3. Have `parseToken` require and parse `gnt`; the authentication middleware invokes both session and grant validators before dispatching any MCP method, including informational tools.
4. Wire `GrantValidator` in `mcp-service/internal/app/app.go` through the existing Redis connection and `authx.MCPGrantStatusKey`.
5. Extend gateway delegated-token validation to require `gnt` and active grant status. Reuse the gateway's Redis connection; do not add a second credential or a direct dependency on auth-service internals.
6. Preserve current public error shape. Do not reveal whether failure came from a missing, revoked, malformed, or unknown grant ID.
7. Ensure audit logs continue to include subject/client/tool/request identifiers but never include `gnt` or bearer values.

**Acceptance criteria:** mcp-service and all `access: delegated` gateway routes fail closed for a missing, inactive, or lookup-error grant; non-delegated browser API routes remain unchanged.

### 4. Fix canonical resource propagation

**Files:** `mcp-service/internal/mcpserver/server.go`, `server_test.go`; optionally extract a small `resource.go` only if it keeps the change readable.

1. Resolve the resource before parsing the bearer token and save it on the authenticated principal.
2. Replace every `s.config.Resource` argument passed to `exchangeDelegatedToken` with `principal.resource`.
3. Keep configured-resource validation behavior unchanged. Reject invalid forwarded values rather than forming a permissive audience.
4. Verify all tool paths, including multi-scope device enrichment and grouped writes, use the same principal resource.
5. Leave `MCP_AUTHORIZATION_SERVER_ISSUER` independent: it controls the issuer used for metadata/token validation, while the resource controls audience and token exchange.

**Acceptance criteria:** an empty `MCP_RESOURCE_URI` with valid forwarded HTTPS headers supports discovery, initialize, read, and delegated exchange using one identical canonical resource string.

### 5. Make discovery/catalog output truthful

**Files:** `mcp-service/internal/mcpserver/server.go`, `server_test.go`, `doc/mcp.md`.

1. Update `describe_homenavi_api` to return only registered callable operations, or split unavailable operations into an explicitly named `planned_capabilities` field whose entries say `not available through MCP`.
2. Change `list_event_channels` to return an empty list with an unavailability explanation, or remove the tool until subscriptions exist. Do not advertise `deferred_typed_subscription` as current delivery.
3. Update `get_write_policy` to describe existing controlled writes only. Remove deferred write labels from the active policy response.
4. Define one catalog source of truth. The preferred small change is a centralized static catalog descriptor used by informational output and tool registration tests; defer a broad registration refactor until this behavior is stable.

**Acceptance criteria:** every advertised active capability maps to a registered tool callable by an appropriately scoped principal, and no discovery response implies a callable event subscription or unavailable write.

### 6. Update current documentation and deployment guidance

**Files:** `doc/mcp.md`, `doc/mcp_connection.md`, `doc/external_api_surface.md`, Helm values comments, deployment documentation where the public issuer/resource is configured.

1. Replace the “Planned Account Connection Removal” section with current behavior: connected applications list/revoke exists, and removal terminates the grant immediately after this plan ships.
2. State that `MCP_RESOURCE_URI` is required until the fallback fix releases; after release, describe it as strongly recommended for reverse-proxy production deployments and required when a distinct public MCP URL is used.
3. State that `MCP_AUTHORIZATION_SERVER_ISSUER` must be explicit when the public issuer cannot be reliably reconstructed from proxy headers. Use the canonical production example with `https://.../api/auth`.
4. Align informational catalog prose with the actual exposed tools and clarify that subscriptions/deferred write categories are unavailable.
5. Keep the 30-day dynamic-client registration TTL statement; it matches current implementation.
6. Keep roadmap documents labeled as planned/historical; do not revise them as though they were operator documentation.

**Acceptance criteria:** no current documentation says Helm/Compose needs no MCP host configuration before the fallback release, and no document describes unimplemented subscriptions or writes as active capabilities.

### 7. Defer structural extraction until behavior is protected

**Files:** `mcp-service/internal/mcpserver/server.go` and new files under the same package.

Perform this after work packages 1–6 are passing. Extract without changing public behavior:

1. `resource.go`: public resource, metadata, issuer, and validation helpers.
2. `authentication.go`: principal parsing and auth/rate/origin middleware.
3. `catalog.go`: tool metadata and informational outputs.
4. Domain registration files by scope family: devices, inventory, history, automation.
5. `audit.go`: request detail parsing and response logging.

Preserve `New(Config)` and the HTTP routes so downstream wiring and test harnesses do not need a broad migration.

## Testing Roadmap

### Layer 1: Unit tests

| Area | Required cases |
| --- | --- |
| Shared grant helpers | key format; non-empty grant ID handling; no duplicated literals |
| Consent manager | grant ID created; grant ID changes on reapproval; revoke deletes status; repeated revoke is safe; subject/client isolation |
| OAuth code and refresh | grant ID is persisted end-to-end; refresh succeeds for matching active grant; refresh fails for revoked/mismatched/missing grant; old records without grant ID fail closed |
| MCP token issuance | `gnt` is present; missing grant ID is rejected; issuer/audience/type/lifetime remain unchanged |
| Token exchange | active matching grant succeeds; inactive/missing/malformed grant fails; wrong scope/audience/session behavior remains covered |
| MCP authentication | missing/revoked grant rejects initialize and tools; validator error fails closed; active grant succeeds; session and origin behavior remains unchanged |
| Gateway middleware | delegated token requires active grant in addition to current type/audience/scope/role/session checks; browser routes ignore the new delegated-only requirement |
| Resource derivation | config wins; forwarded HTTPS derivation; first forwarded value; malformed host/scheme rejection; direct TLS fallback |
| Catalog | every advertised active item is registered; unavailable capabilities are explicitly unavailable; scopes filter catalog and registered tools consistently |

### Layer 2: Handler/component tests

1. **Blank-resource authenticated read regression** in `mcp-service/internal/mcpserver/server_test.go`:
   - blank `MCP_RESOURCE_URI`;
   - forwarded `https` host;
   - MCP token audience and issuer matching the derived resource;
   - fake auth-service captures token-exchange `resource` and scope;
   - fake gateway/device route returns a fixture;
   - `list_devices` succeeds and exchange receives the derived resource exactly.
2. **Consent-revocation request regression**:
   - issue an MCP token and execute initialize/tool successfully;
   - remove its grant key while leaving login session active;
   - same token receives unauthorized response for initialize and a domain tool;
   - a second client grant for the same subject remains accepted.
3. **Gateway immediate-revocation regression**:
   - issue delegated token;
   - remove grant key before its two-minute expiry;
   - delegated route rejects it;
   - grant-status validator errors reject it without proxying downstream.
4. **Catalog contract test**:
   - compare tool-list output with catalog response for a principal holding all scopes;
   - verify none of the deferred operations/channel names appear as active capabilities.

### Layer 3: Service integration tests

Add or extend the existing Compose/integration profile to cover this sequence:

1. Dynamic-register client A and client B for the same resident user.
2. Complete PKCE and obtain an MCP/refresh token for A.
3. Execute a read tool through the public gateway.
4. Revoke A through the authenticated consent endpoint.
5. Assert A cannot refresh, initialize, call a tool, exchange a delegated token, or use a previously issued delegated token.
6. Assert B and the browser session still work.
7. Reapprove A; assert only a newly issued token works.
8. Repeat the public read path with `MCP_RESOURCE_URI` intentionally blank and correct forwarded headers.

Run these tests against Redis, auth-service, mcp-service, api-gateway, and a deterministic fixture device service. Do not mock the token exchange or route authorization in this layer.

### Layer 4: Deployment smoke tests

Before release:

1. Render Helm with explicit resource and issuer; assert both services receive canonical values.
2. Render Helm with a blank resource; run the authenticated read regression through the ingress/proxy fixture.
3. Check protected-resource metadata, authorization-server metadata, authorization endpoint, token endpoint, and JWKS all advertise HTTPS public URLs.
4. Confirm `MCP_ENABLED=false` still hides metadata and transport.
5. Run a production-like Open WebUI OAuth refresh smoke test to ensure metadata issuer changes do not reintroduce the prior `http` endpoint/`405` failure.

## Release and Rollout

1. Release shared-claim, auth-service, mcp-service, and api-gateway changes together. Do not deploy one validator before issuers emit `gnt`.
2. Use a short compatibility window only if a rolling deployment requires it: issue new tokens with `gnt` first, then require it at validation boundaries. Do not accept missing `gnt` after the rollout completes.
3. During rollout, force clients with pre-change MCP tokens to reauthorize or wait for their one-hour expiry. Refresh tokens without a grant ID must receive an authorization-required response, not be silently upgraded.
4. Deploy explicit `MCP_RESOURCE_URI` and `MCP_AUTHORIZATION_SERVER_ISSUER` values in production before enabling the blank-resource fallback test path.
5. Monitor counts of rejected grant validations, token-exchange failures, gateway delegated denials, and OAuth refresh failures. Log only reason categories, never grant IDs or bearer tokens.
6. Validate Connected Applications removal manually with two clients before declaring the release complete.

## Completion Criteria

The work is complete when all of the following are true:

- Removing client A immediately blocks A at MCP and gateway boundaries without affecting client B or the browser session.
- Reapproving A does not reactivate old MCP or delegated tokens.
- Blank `MCP_RESOURCE_URI` supports an authenticated read through the full delegated path under valid forwarded HTTPS headers.
- MCP catalog/tool documentation contains no active claim for unavailable writes or subscriptions.
- Current docs describe the real revocation and configuration semantics.
- Focused unit/component tests, service integration tests, deployment smoke tests, and the existing MCP/OAuth suites all pass.