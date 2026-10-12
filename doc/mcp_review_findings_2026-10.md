# MCP Review Findings (2026-10)

## Scope and Method

This is a review-only report. No production behavior is changed by this document.

Reviewed areas:

- `mcp-service/internal/mcpserver/server.go` and its tests
- OAuth authorization, consent, refresh, and delegated-token paths in `auth-service`
- MCP resource/issuer configuration in Compose, Helm, and GitOps-facing values
- Current MCP documentation in `doc/mcp.md`, `doc/mcp_connection.md`, and `doc/external_api_surface.md`

Focused tests passed:

```text
go test ./mcp-service/internal/mcpserver ./mcp-service/internal/app ./auth-service/internal/auth ./auth-service/internal/http/oauth
```

## Summary

| Finding | Assessment | Priority |
| --- | --- | --- |
| `server.go` combines transport, authentication, metadata, auditing, and tool registration | Valid maintainability concern; not a current authorization bypass | Medium |
| The MCP catalog describes deferred writes and subscriptions as if usable | Confirmed documentation/tool-contract issue | High for agent usability |
| Consent revocation does not invalidate active MCP access tokens or delegated exchange | Confirmed security gap | High |
| Blank `MCP_RESOURCE_URI` derives metadata correctly but fails domain-tool delegation | Confirmed functional configuration bug | High |
| MCP docs describe some implemented account features as planned and overstate fallback safety | Confirmed documentation drift | Medium |

## 1. Concentration of Responsibilities

### Finding

This concern is valid. `mcp-service/internal/mcpserver/server.go` currently owns:

- HTTP routes and Streamable HTTP handler setup
- protected-resource metadata and canonical-resource derivation
- bearer parsing, JWT validation, role/scope checks, origin enforcement, and session validation
- rate limiting and request audit logging
- the scoped tool catalog and every tool implementation closure
- write-policy validation

The file is already large, and a security-sensitive change to one of these areas requires working around unrelated tool-registration code. `devices.go` and `read_clients.go` are useful existing extraction boundaries, but they do not separate the transport/security boundary from registration policy.

### What is safe today

This is not evidence of a current authorization defect. The central guard has meaningful focused tests for token type, audience, role, origin, session revocation, rate limiting, and scoped discovery. The handler also keeps raw bearer tokens out of audit logs.

### Recommendation

Refactor only when making a related feature change. Suitable ownership boundaries are:

1. `resource.go`: public resource derivation, resource metadata, issuer derivation, and validation.
2. `authentication.go`: bearer parsing, principal construction, session validation, origin policy, and rate limiting.
3. `audit.go`: request parsing and structured completion logging.
4. `catalog.go`: shared tool metadata and scope/feature-flag policy.
5. domain registration modules such as `register_devices.go`, `register_inventory.go`, `register_automation.go`, and `register_history.go`.

Keep the middleware test suite at the public `/mcp` boundary; moving code without preserving those integration-style tests would reduce confidence.

## 2. Catalog and Capability Documentation

### Finding

This is a confirmed issue. The code exposes three informational tools to every authenticated MCP principal:

- `describe_homenavi_api`
- `list_event_channels`
- `get_write_policy`

`describe_homenavi_api` reports deferred writes (`pairing`, automation changes, dashboard changes, and user/account changes) and raw-looking event channel paths. `list_event_channels` reports `deferred_typed_subscription` delivery for automation, registry, and device changes.

None of those deferred writes or subscription mechanisms are registered as executable MCP tools. There is no MCP `subscribe` tool, event delivery transport, or raw WebSocket/MQTT forwarding tool. In contrast, `doc/mcp.md` correctly says those changes remain unavailable through MCP.

An agent can reasonably interpret `describe_homenavi_api` and `list_event_channels` as actionable capability discovery, so this is more than prose drift: it is an inaccurate tool contract.

### What is safe today

The misleading catalog does not expose a raw event transport or an unbounded write operation. Scope gates, feature flags, and the absence of a registration handler prevent the claimed operations from being called.

### Recommendation

Choose one clear contract and test it:

- **Preferred:** remove unavailable deferred writes/channels from executable catalog tools, or return them under an explicit `planned_capabilities` field with wording such as `not available through MCP`.
- **Alternative:** implement typed subscription tools and document their lifecycle, delivery, cancellation, retention, reconnect, and scope behavior.
- Derive the informational catalog from the actual registration policy, or add a test asserting that every advertised capability maps either to a registered callable tool or to an explicitly unavailable/planned status.

The write-policy response should also avoid implying a future approval system is active. `approval_required: false` is accurate for current write tools, but the surrounding deferred-write list makes that distinction easy to miss.

## 3. Consent Revocation and Active Tokens

### Finding

This is a confirmed security gap.

Revoking consent through `DELETE /api/auth/oauth/consents/{clientID}` deletes the persistent consent record. It has these current effects:

| Flow | Current behavior after consent revocation |
| --- | --- |
| New authorization-code issuance without fresh approval | Rejected because `AuthorizeOAuthForSession` checks consent; the user may explicitly grant it again |
| OAuth refresh-token grant | Rejected because `RefreshOAuthAuthorizationGrant` checks consent |
| Existing MCP access token | Still accepted until its one-hour JWT expiry if the browser/session remains active |
| Delegated token exchange from that existing MCP token | Still accepted until the MCP token expires; `ExchangeMCPAccessToken` checks token scope and active session, not consent |
| Already-issued delegated token | Remains usable for its two-minute JWT lifetime, subject to the existing active-session checks in downstream services |

The problem is that revocation is keyed by `(user, client)`, while the live MCP request path validates only token claims plus the broad login-session status. Revoking one connected application must not normally revoke the user's complete browser session, so the existing session-status key cannot represent client-specific consent revocation.

### Existing coverage

- Consent manager tests cover scope binding and deletion.
- OAuth refresh tests cover rotation, replay, active-session binding, and consent checks during refresh.
- MCP tests cover a revoked **login session**.

There is no test proving that consent revocation rejects a still-valid MCP access token or its delegated exchange. The `doc/mcp.md` account-removal section describes immediate active-token termination as planned behavior, not behavior that is implemented today.

### Recommendation

Define and implement a client-specific grant lifecycle. The recommended contract is:

1. Consent removal immediately rejects new authorization codes, refresh grants, MCP requests, and delegated exchange for the `(subject, client_id, resource)` grant.
2. Already-issued delegated tokens may retain their existing two-minute bounded lifetime, or may be rejected immediately if downstream authorization can cheaply consult the same grant state. Document the chosen behavior.
3. Do not revoke the user's entire browser session merely to disconnect one client.

A practical design is a server-side grant record or consent-version/revoked-at value carried in the MCP token and checked by mcp-service and auth-service token exchange. Maintain an index from subject/client/resource to refresh-token families so revocation also invalidates those families. Token `jti` values can support an immediate deny list, but a versioned grant check avoids tracking every individual MCP token.

Required regression coverage:

- revoke then authorize: reject
- revoke then refresh: reject
- revoke then `/mcp` with an otherwise valid MCP token: reject
- revoke then delegated token exchange: reject
- defined behavior for an already-issued delegated token
- revoking client A does not disconnect client B or the browser session

## 4. Canonical MCP Resource Configuration

### Finding

This is a confirmed functional bug, not merely a missing test.

When `MCP_RESOURCE_URI` is blank, `mcp-service` correctly derives and validates a canonical resource from `X-Forwarded-Host` and `X-Forwarded-Proto`. That resource is used for protected-resource metadata and MCP bearer-token validation.

However, domain-tool closures use `s.config.Resource` directly when asking auth-service for a delegated token. With blank configuration, that value is still empty. `readClients.exchangeDelegatedToken` sends the empty value in the OAuth token-exchange `resource` form field, while auth-service validates the MCP token audience against that field. The derived public audience cannot equal an empty audience, so a real read tool fails before calling the gateway.

The existing `TestMCPDerivesResourceFromForwardedRequest` checks only metadata. It does not execute an authenticated read tool with a token whose audience is the derived public resource.

### Operational consequence

The public resource must currently be configured explicitly for working delegated tools. The production GitOps deployment already pins both:

```text
MCP_RESOURCE_URI=https://home.homenavi.org/mcp
MCP_AUTHORIZATION_SERVER_ISSUER=https://home.homenavi.org/api/auth
```

The explicit issuer is additionally important when a proxy cannot reliably preserve the public scheme for authorization-server metadata. A previous production refresh failure demonstrated this: auth-service without the explicit issuer advertised an `http` token endpoint behind HTTPS, which Open WebUI followed and received `405`.

### Recommendation

Resolve the effective canonical resource once in authentication middleware, store it in the authenticated request/principal context, and pass that value to every delegated exchange. Do not read `s.config.Resource` directly from tool closures.

Add an end-to-end handler regression test with:

1. blank `MCP_RESOURCE_URI`
2. `X-Forwarded-Host` and `X-Forwarded-Proto=https`
3. an MCP token whose audience is the derived `https://host/mcp` resource
4. an actual read tool such as `list_devices`
5. a fake auth-service asserting that the token exchange receives the same derived resource
6. a fake gateway returning a device result

## Documentation Updates Needed

The following changes are needed for documentation to describe the current codebase accurately:

1. `doc/mcp.md`: remove or relabel the catalog's deferred writes/event channels as unavailable. Keep the current write-tool table as the source of truth.
2. `doc/mcp.md`: replace the **Planned Account Connection Removal** section. The connected-applications UI and consent list/revoke endpoints exist. State the actual current revocation boundary, then update it again when immediate client-specific invalidation is implemented.
3. `doc/mcp.md`: do not state that Compose and Helm need no MCP-specific host configuration. Until the blank-resource delegation defect is fixed, document explicit `MCP_RESOURCE_URI` as required for any deployment that uses domain tools. Recommend explicit `MCP_AUTHORIZATION_SERVER_ISSUER` whenever proxy headers cannot be guaranteed end-to-end.
4. `doc/mcp_connection.md`: retain its public-HTTPS and proxy-header guidance. Its statement that dynamic client registrations expire after 30 days is accurate: dynamic-client records are stored with a 30-day TTL.
5. `doc/external_api_surface.md`: its listed OAuth, MCP, and consent endpoints match the reviewed code, but it should link to the clarified revocation semantics after the documentation above changes.
6. Historical roadmap documents should remain clearly marked as historical/planning material. They should not be treated as a statement of deployed behavior.

## Conclusion

The four original observations were well-founded:

- responsibility concentration is a justified refactoring target, but not an active security flaw;
- catalog wording is misleading even though the unavailable capabilities are safely not exposed;
- consent revocation is not currently immediate for active MCP access and needs client-specific invalidation;
- blank resource configuration is an actual delegated-tool failure path and needs both a code fix and a regression test.

The report intentionally does not prescribe a broad refactor before the two correctness/security fixes. The highest-value implementation order is: canonical-resource propagation, client-specific consent invalidation, catalog truthfulness, then modularization as part of subsequent MCP capability work.