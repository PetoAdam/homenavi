# Connect an MCP Client

Homenavi exposes a remote Streamable HTTP MCP server at:

```text
https://<homenavi-host>/mcp
```

Use the public HTTPS address that users open in a browser. The MCP client must be able to reach this address, including its OAuth metadata endpoints. Do not use an internal container address such as `http://mcp-service:8080` from a remote client.

## Client Setup

1. In the MCP client, add a **Streamable HTTP** server.
2. Set its server URL to `https://<homenavi-host>/mcp`.
3. Select OAuth 2.1 authentication with Dynamic Client Registration (DCR), when the client asks.
4. Start the connection. The client discovers Homenavi's protected-resource and authorization-server metadata, registers itself, and opens the Homenavi sign-in and consent flow.
5. Sign in as a resident or administrator and review the requested scopes before approving the connection.

For example, an Open WebUI connection uses the Streamable HTTP transport, the Homenavi `/mcp` URL, OAuth 2.1 authentication, and DCR. No Homenavi-specific client ID, client secret, or callback allowlist entry is required.

## Dynamic Client Registration Policy

Homenavi supports public MCP clients through OAuth Dynamic Client Registration. A dynamically registered client may use:

- An absolute `https://` redirect URI for a web, hosted, or app-claimed HTTPS client.
- An `http://localhost`, `http://127.0.0.1`, or `http://[::1]` loopback redirect URI for a native desktop client.

Homenavi rejects remote HTTP redirects, URI fragments, embedded credentials, and malformed callback URIs. HTTPS redirect URIs must match the URI registered during DCR exactly at authorization time. Native loopback clients may use a different ephemeral port, as permitted for native OAuth clients.

This policy is generic: adding a new AI client does not require a code or deployment change. Dynamic registration entries expire after 30 days. Authorization still requires PKCE S256, a state value, an MCP resource indicator, an authenticated user, and explicit user consent.

## Scopes and Access

The consent screen identifies the MCP client and requested Homenavi scopes. Grant only the access it needs. Read scopes allow discovery and state inspection; write or execution scopes can control devices, create groups, or run existing workflows. See [MCP](mcp.md) for the scope-to-tool mapping and the write policy.

An approved client receives an access token bound to Homenavi's public `/mcp` resource. It cannot use that token as a general Homenavi API token.

## Troubleshooting

- **Registration failed:** confirm the client supplied an HTTPS callback or a loopback HTTP callback. A remote `http://` callback is intentionally rejected.
- **Browser cannot complete sign-in:** confirm the public reverse proxy preserves `Host` and `X-Forwarded-Proto` so OAuth metadata and consent redirects use the public HTTPS origin.
- **MCP URL does not authenticate:** use `/mcp`, not a legacy SSE endpoint, and allow the client to perform OAuth discovery rather than supplying a manually copied bearer token.
- **Tool is unavailable:** approve the required scope, then reconnect. Homenavi only advertises tools allowed by the approved scopes and optional operator tool policy.

For deployment requirements, see [Deployment Guide](deployment_guide.md). For protocol details, tool behavior, and operations, see [MCP](mcp.md).