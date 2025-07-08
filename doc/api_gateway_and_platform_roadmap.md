# Homenavi API Gateway & Platform Roadmap

## 1. Infrastructure & Security
- [x] Deploy behind Cloudflare for DDoS protection and TLS termination.
- [ ] Add Nginx as a reverse proxy:
  - [ ] Enforce HTTPS.
  - [ ] Route `/` to frontend, `/api` to API Gateway.
  - [ ] Harden Nginx config (security headers, rate limiting, etc.).

## 2. Authentication & Authorization
- [ ] Refactor Auth Service:
  - [ ] Use strong JWT signing (RS256 or rotating secrets).
  - [ ] Implement JWT refresh tokens.
  - [ ] Add OAuth 2.0 (Google) login support.
  - [ ] Add 2FA (email-based, later TOTP).
  - [ ] Add token revocation/blacklist support.
- [ ] Refactor User Service:
  - [ ] Store user profiles, OAuth identities, 2FA secrets.
  - [ ] Expose endpoints for user management and 2FA setup/verification.

## 3. API Gateway Features
- [x] Dynamic routing via YAML.
- [x] Per-endpoint and global rate limiting.
- [x] JWT-based access control.
- [x] Observability: Prometheus metrics, Jaeger tracing.
- [ ] WebSocket proxy support:
  - [ ] Proxy and upgrade WebSocket connections.
  - [ ] Add OpenTelemetry tracing for WebSocket events.
- [ ] Prefix-based routing:
  - [ ] Allow route definitions with path prefixes (e.g., `/api/devices/*`).
  - [ ] Support path parameter replacement in prefix routes.
- [ ] OpenAPI integration:
  - [ ] Convert OpenAPI specs to gateway YAML routes.
  - [ ] (Optional) Serve OpenAPI docs for all registered routes.

## 4. Observability & Monitoring
- [x] Prometheus metrics for all endpoints.
- [x] Distributed tracing with Jaeger.
- [ ] Extend tracing to WebSocket events.
- [ ] Add custom metrics for authentication, errors, and WebSocket usage.
- [ ] Provide example Grafana dashboards.

## 5. Developer Experience
- [x] `.env.example` for easy config.
- [x] Modular codebase for easy extension.
- [ ] CLI tool for managing routes and config.
- [ ] Hot-reload support for gateway config/routes.

## 6. Documentation & Community
- [x] Update README with observability and security features.
- [ ] Add guides for:
  - [ ] Nginx + HTTPS setup.
  - [ ] Adding new microservices.
  - [ ] Using OpenTelemetry and tracing.
  - [ ] OpenAPI integration.
- [ ] Example projects and recipes.