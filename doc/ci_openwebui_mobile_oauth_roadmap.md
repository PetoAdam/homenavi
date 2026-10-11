# CI, Open WebUI, and Mobile OAuth Roadmap

## Objective

Restore the two failing CI jobs, make the Open WebUI Homenavi MCP connection obtain and exercise write scopes, and support secure OAuth public clients for the native mobile application.

## Current Findings

- [x] Ollama was updated to `0.40.2`; `qwen3:14b` can issue a valid `read_file` tool call through the Ollama API.
- [x] The authorization-server metadata already advertises `token_endpoint_auth_methods_supported: ["none"]` and `code_challenge_methods_supported: ["S256"]`.
- [x] Dynamic clients are currently granted the Homenavi write scopes, including `home.devices.write` and `home.automation.execute`.
- [x] Authorization requires an S256 challenge and code exchange verifies the S256 verifier.
- [x] Dynamic registration persists client type and token endpoint authentication method.
- [x] Dynamic registration accepts safe native custom redirect schemes while rejecting unsafe standard schemes.
- [ ] The current host does not run an Open WebUI container, so its deployed configuration has not yet been inspected.
- [ ] GitHub Actions logs require an authenticated GitHub CLI or token in this environment; anonymous log download is denied with HTTP 403.

## Workstream 1: CI Failures

### Evidence Collection

1. Authenticate `gh` using an existing GitHub CLI session or a user-entered token. Do not place a token in repository files, terminal history, or chat messages.
2. Retrieve failed-step logs for jobs `113870726051` and `113870727819`.
3. Record the workflow name, failing command, failing test, relevant stack trace, and commit SHA in this document.
4. Reproduce only the failing command locally before changing production code.

### Repair and Validation

1. Make the smallest root-cause fix in the owning package.
2. Add or adjust a focused regression test for each repaired behavior.
3. Run the focused test first, then the workflow-equivalent local command.
4. Re-run the affected GitHub Actions workflow and attach the successful job URLs to the implementation record.

Status: partially resolved.

- Auth Service job `113870727819`: the public annotation identified a syntax error caused by a test declaration nested inside another test at commit `8cd0d33`. The current working tree has the declaration at package scope; `go vet ./...` and `go test ./...` pass from `auth-service`.
- HA job `113870726051`: the public annotation identifies the `Install HA profile` Helm step, but the logs and failed-pod diagnostics require GitHub sign-in. Local Helm lint and rendering pass and the HA values include the required dependency overrides. Do not change chart behavior without the failed workload diagnostics.

## Workstream 2: Native Mobile Public Clients

### Registration Contract

1. Extend the dynamic-registration model to persist `application_type` and `token_endpoint_auth_method` with the registered client.
2. Accept only `token_endpoint_auth_method: "none"` for dynamic clients. Omitted input normalizes to `none` for backwards compatibility.
3. Require `application_type` to be `native` or `web`; normalize omitted input to `native` only where that matches the existing registration behavior.
4. Return the normalized values from `/api/auth/oauth/register`.

### Redirect URI Policy

1. Continue allowing HTTPS redirects for web and native clients.
2. Continue allowing loopback HTTP redirects only for native clients, including variable loopback ports when the registered URI has the same host and path.
3. Allow custom schemes only for native clients.
4. Reject unsafe schemes, fragments, user-info, empty schemes, opaque URIs, and custom schemes that cannot safely receive an OAuth callback.
5. Require an exact match for custom-scheme redirects during authorization; loopback port relaxation must not apply to custom schemes.
6. Add positive coverage for `mcp-agent://callback` and negative coverage for custom web-client, malformed, and unsafe redirects.

### PKCE Policy

1. Keep S256 mandatory for every authorization-code request, including all public clients.
2. Keep verifier validation and constant-time S256 comparison at the token exchange.
3. Add registration and authorization tests proving a public native client cannot authorize or redeem a code without valid S256 PKCE.
4. Document the mobile integration requirement: generate a high-entropy verifier, derive the S256 challenge, and never ship a client secret.

### Compatibility and Rollout

1. Preserve existing dynamic-client cache serialization compatibility by treating omitted newly added fields as `native` and `none` on reads.
2. Add a migration-free rollout test using an existing serialized dynamic client record.
3. Update OAuth metadata and external API documentation only if the normalized registration contract changes.
4. Run `go test ./auth-service/...` before rollout.

Status: implemented and locally validated.

- Dynamic client registration now records `application_type` and `token_endpoint_auth_method`, normalizing legacy cached records to `native` and `none`.
- Native public clients can register an exact custom callback such as `mcp-agent://callback`; web clients, opaque URIs, fragments, user-info, and unsafe standard schemes remain rejected.
- Existing S256 authorization and token-exchange enforcement remains in place.
- Focused OAuth tests and `go test ./auth-service/...` pass.

## Workstream 3: Open WebUI MCP Write Scopes

### Inspect the Deployed Connection

1. Identify the Open WebUI deployment host, version, and configuration source (Docker Compose, Helm, or managed instance).
2. Capture the MCP server definition, OAuth discovery URL, registration request, requested authorization scopes, and stored connection state with secrets redacted.
3. Compare the requested scopes and received token `scope` claim against the server-supported scopes.

### Configure and Upgrade

1. Upgrade Open WebUI using its supported deployment mechanism after reviewing release notes for MCP OAuth and scope-handling changes.
2. Configure the Homenavi MCP connection to request the least necessary write scopes: `home.devices.read`, `home.devices.write`, and `home.automation.execute`; include inventory/history scopes only if required by tools used in the chat.
3. Remove the stale OAuth registration/connection state so Open WebUI performs a fresh DCR and consent flow.
4. Complete the authorization flow using the existing browser session; do not expose credentials to chat or repository files.

### End-to-End Validation

1. Use GPT-5.6 Luna with the Homenavi MCP connection.
2. Read the Bedroom Wardrobe state and capture its original state.
3. Send an idempotent command to turn it on, verify the state changes, then restore its original state and verify again.
4. Record the exact granted token scopes and the MCP tool-call outcomes in this document.

Status: partially implemented.

- The live instance reports Open WebUI `0.9.6`; GitOps now pins both its application and init-container images to the latest stable `v0.11.4`.
- The init-container Python indentation was corrected so a fresh rollout can persist the public URL instead of failing before Open WebUI starts.
- A 64-byte random `WEBUI_SECRET_KEY` now exists only in the Kubernetes `ai/open-webui-secrets` Secret. The GitOps deployment references it and enables encrypted tool values and secure session cookies; the secret value is not present in Git.
- Open WebUI `v0.11.4` is deployed through Argo CD at GitOps revision `51e7c230dbc6fe1393cad8db34c02e6ac9114e52`. The deployment is `Synced` and `Healthy`; the public version endpoint returns `0.11.4` and the secret-backed encryption and secure-cookie runtime settings are active.
- Live validation completed through an authenticated Open WebUI browser session. The selected Homenavi MCP connection successfully completed OAuth and exposed the read tools `list_devices`, `get_device`, `get_device_state`, `list_device_groups`, `get_device_group`, `get_device_group_state`, `list_device_integrations`, `list_pairings`, `list_rooms`, `list_automation_workflows`, `get_automation_workflow`, and `query_state_history`.
- The previous Open WebUI session was invalidated by the upgrade. Sign in directly through the browser, complete a fresh Homenavi OAuth authorization, and confirm `send_device_command` is exposed before running the safe Bedroom Wardrobe on/verify/restore test. No device write has been attempted.

## Workstream 4: OAuth and MCP Security Standards

### Verified Controls

- [x] RFC 8252 native-app behavior: native clients use authorization code flow, S256 PKCE is mandatory, loopback HTTP is limited to loopback hosts, and custom callbacks must match exactly.
- [x] RFC 7636: code challenges and verifiers are length/character constrained, S256 is required, and the verifier digest is checked with constant-time comparison.
- [x] OAuth resource indicators (RFC 8707): authorization and token exchange bind the resource to the `/mcp` audience.
- [x] Authorization-server metadata advertises the issuer, authorization/token/JWKS/registration endpoints, S256, public-client authentication, and supported scopes.
- [x] MCP protected-resource metadata and `WWW-Authenticate` challenges advertise the protected resource and authorization server.
- [x] MCP uses stateless Streamable HTTP, validates RS256 issuer/audience/key ID/token type/session/role, applies origin restrictions and rate limiting, and scope-gates tool discovery and execution.
- [x] Controlled writes require a scope, an online/capability check, an idempotency key, delegated access tokens, and structured audit logging.

### Remaining Security Decisions

1. Device writes currently execute after client-side confirmation and do not require a separate server-side approval artifact. This is compatible with MCP, but high-risk device classes should eventually require a short-lived user confirmation token or policy decision recorded server-side.
2. The requested `mcp-agent://callback` custom scheme is supported for native clients. RFC 8252 recommends reverse-domain custom schemes when possible; prefer an app-claimed HTTPS redirect or an application-owned reverse-domain scheme for the native app when its bundle identifier is finalized.
3. Add an interoperability test that exercises discovery, DCR, authorization, token exchange, tool discovery, and a non-destructive write against a deployed environment after the auth-service image containing these changes is released.

## Implementation Log

| Date | Work item | Result |
| --- | --- | --- |
| 2026-10-10 | Baseline inspection | Confirmed S256 enforcement and dynamic write-scope grants; identified custom-scheme and metadata persistence gaps. |
| 2026-10-10 | Ollama validation | Updated to `0.40.2`; Qwen tool-call test passed. |
| 2026-10-10 | Mobile public OAuth clients | Implemented public-client metadata persistence, safe native custom callbacks, and compatibility defaults; focused and full auth-service tests passed. |
| 2026-10-10 | Auth CI diagnosis | Identified the nested test declaration at `8cd0d33`; current `go vet ./...` and `go test ./...` pass. |
| 2026-10-10 | Open WebUI GitOps | Updated `v0.9.6` to `v0.11.4` and repaired init-container script indentation; reconciliation remains blocked by unavailable cluster API. |
| 2026-10-10 | Open WebUI secret protection | Created a Kubernetes-only 64-byte `WEBUI_SECRET_KEY`; added its GitOps reference plus encrypted tool values and secure session cookies. |
| 2026-10-10 | Standards audit | Verified RFC 8252/7636/8707 and MCP protected-resource controls; documented deferred server-side write approval policy. |