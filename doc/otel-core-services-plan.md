# Homenavi OTEL Core Services Plan

## Goal
Add first-class OpenTelemetry coverage to the remaining core services so tracing, metrics, and request correlation work consistently across the platform.

## Current established pattern
The repo already has a common observability path:
- `shared/observability.SetupObservability(serviceName)` configures the OTEL propagator, Prometheus exporter, meter provider, tracer provider, and the `Trace-ID` response header.
- `shared/observability.MetricsAndTracingMiddleware(tracer, serviceName)` wraps HTTP handlers and creates request spans plus request metrics.
- Instrumented services already follow this pattern in their app bootstrap and router wiring.

Services that already use the shared pattern include:
- `api-gateway`
- `auth-service`
- `user-service`
- `dashboard-service`
- `device-hub`
- `email-service`
- `mock-adapter`
- `zigbee-adapter`

Services that still need first-class OTEL support:
- `automation-service`
- `entity-registry-service`
- `history-service`
- `integration-proxy`
- `weather-service`

Auxiliary services that may be added after the core set:
- `profile-picture-service`
- `echo-service`

## Standard service pattern
Every HTTP service should use the same bootstrap shape:
1. Call `shared/observability.SetupObservability(serviceName)` during app startup.
2. Pass the returned tracer and Prometheus handler into the router.
3. Wrap the router with `shared/observability.MetricsAndTracingMiddleware`.
4. Expose `/metrics` from the shared Prometheus handler.
5. Keep `/health` and other readiness endpoints outside expensive tracing logic where practical.

The plan should not introduce a second observability convention. The shared package is the canonical entry point.

## What should be instrumented

### HTTP ingress
- Request spans for all public HTTP handlers.
- Standard span attributes: method, target path, service name, status code, request ID.
- Trace context propagation from incoming headers.

### Outbound calls
- HTTP client calls should carry trace context.
- DB access should emit spans or use a standard instrumentation wrapper where feasible.
- Redis operations should be observable for hot-path services.
- MQTT interactions should carry span context or explicit correlation IDs when messages cross service boundaries.

### Background work
- Cron-like loops, polling loops, and async workers should create spans for the parent task and the inner units of work.
- Long-lived handlers should emit spans for retry, backoff, and error paths.

## Service-by-service rollout

### automation-service
- Add the shared observability bootstrap in app startup.
- Wrap HTTP routes with the shared middleware.
- Add spans around workflow execution, MQTT subscribe callbacks, and publish operations.
- Instrument any background engine loops separately from request handling.

### entity-registry-service
- Add bootstrap and router middleware.
- Trace the autoimport runner and its MQTT consumption path.
- Add spans around registry refresh, metadata merges, and write operations.

### history-service
- Add bootstrap and router middleware.
- Trace ingest handlers, list/query handlers, and MQTT event ingestion.
- Add spans around database writes and query windows.

### integration-proxy
- Add bootstrap and router middleware.
- Trace install, uninstall, update, reload, and runtime-status handlers.
- Add spans around shell command execution, config parsing, and marketplace fetches.

### weather-service
- Add bootstrap and router middleware.
- Trace upstream weather fetches, cache hits/misses, and transformation logic.

## Implementation phases

### Phase 1: shared bootstrap everywhere
- Ensure every core HTTP service uses the same setup path.
- Remove any ad hoc local observability wiring if it duplicates the shared package.
- Keep the Prometheus endpoint convention consistent.

### Phase 2: internal spans
- Add spans to background jobs, MQTT handlers, and expensive service methods.
- Make sure errors are recorded on the span before returning.

### Phase 3: outbound instrumentation
- Add standard wrappers for HTTP clients and any service-specific DB/Redis access.
- Ensure trace context is carried into downstream services.

### Phase 4: dashboards and verification
- Verify traces appear end-to-end across gateway -> service -> downstream calls.
- Verify each service exports request metrics and trace IDs consistently.
- Build a small Grafana/Tempo dashboard set for the core plane.

## Acceptance criteria
- Every core service has the same OTEL bootstrap and request middleware pattern.
- Missing services no longer run without traces or request metrics.
- Requests can be correlated end-to-end across services using the same trace context.
- Background jobs and message consumers have explicit spans instead of silent work.

## Recommendation
Treat `shared/observability` as the only supported observability entry point for HTTP services. That gives the repo one established way to do OTEL instead of several inconsistent ones.
