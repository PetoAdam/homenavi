# Homenavi Go Service Structure Conventions

## Purpose

This document defines the recommended long-term structure for all Go services in the Homenavi monorepo.

It exists to solve the problems already visible in the codebase:

- very large files with mixed responsibilities
- random placement of structs, interfaces, and logic in the same file
- inconsistent package names and folder layouts between services
- weak dependency boundaries between transport, business logic, and infrastructure
- difficulty testing code without booting real databases, MQTT brokers, or HTTP clients

The goal is not to force every service into heavy architecture for its own sake.
The goal is:

- consistent layout across services
- explicit dependency injection
- smaller focused files
- clearer package boundaries
- easier testing
- easier onboarding
- easier future refactors

These conventions apply to all Go services in the repo, including:

- `api-gateway`
- `auth-service`
- `automation-service`
- `dashboard-service`
- `device-hub`
- `email-service`
- `entity-registry-service`
- `history-service`
- `integration-proxy`
- `mock-adapter`
- `user-service`
- `weather-service`
- `zigbee-adapter`

They also apply to future Go services and integrations unless there is a strong reason to diverge.

---

## Core principles

### 1. Organize by capability, not by technical layer alone

Prefer packages that represent service capabilities or domain concepts.

Good:

- `internal/users`
- `internal/devices`
- `internal/installations`
- `internal/automation`
- `internal/catalog`

Avoid global buckets like:

- `internal/models`
- `internal/services`
- `internal/utils`
- `internal/helpers`

These become dumping grounds over time.

A package should answer one clear question:

- Is this transport?
- Is this business logic?
- Is this persistence?
- Is this messaging?
- Is this application wiring?

If the answer is “several unrelated things”, the package is too broad.

### 2. Keep transport, business logic, and infrastructure separate

At minimum, each service should separate:

- startup/wiring
- HTTP handlers / routes
- business logic
- persistence / external integrations

This makes the code easier to test and prevents large files from turning into “everything objects”.

### 3. Use manual dependency injection

Homenavi should prefer explicit constructor-based dependency injection.

Go works very well with manual DI.
A container is not required for most services and would add hidden behavior.

Prefer:

- constructors like `NewService(...)`
- constructors like `NewRepository(...)`
- constructors like `NewHandler(...)`
- one explicit composition root in `cmd/...` or `internal/app`

Avoid:

- hidden globals
- package-level singleton state
- service locators
- reflection-based runtime dependency injection

### 4. Keep files small and focused

One file should usually have one main responsibility.

Bad signs:

- one file contains DTOs, domain types, validation, routes, repository logic, and MQTT publishing
- one file exceeds 500 lines because it contains unrelated responsibilities
- one file requires scrolling through multiple different concerns to understand anything

Recommended limits:

- ideal: under 200 lines
- normal: under 300 lines
- split strongly recommended: above 400 lines
- avoid entirely: 1000+ line files unless generated

### 5. Export only what must cross package boundaries

Default to unexported names unless another package truly needs access.

Export only:

- stable package APIs
- interfaces consumed outside the package
- core types that must cross package boundaries

Keep internal details unexported to reduce surface area and coupling.

### 6. Prefer narrow interfaces at boundaries

Interfaces should describe real seams in the system:

- repositories
- MQTT publishers/subscribers
- HTTP clients
- external service clients
- clocks, ID generators, or transaction boundaries when needed for tests

Avoid creating interfaces for every struct automatically.
In Go, interfaces are most useful at package boundaries, not as a blanket pattern.

---

## Standard service layout

This is the default target layout for medium-sized Homenavi Go services.
Not every service needs every folder, but every service should follow the same ideas.

```text
<service>/
├─ cmd/
│  └─ <service>/
│     └─ main.go
├─ internal/
│  ├─ app/
│  │  ├─ app.go
│  │  ├─ config.go
│  │  └─ wiring.go
│  ├─ http/
│  │  ├─ router.go
│  │  ├─ middleware.go
│  │  └─ <capability>_handler.go
│  ├─ infra/
│  │  ├─ db/
│  │  ├─ mqtt/
│  │  ├─ redis/
│  │  ├─ clients/
│  │  └─ observability/
│  ├─ <capability-a>/
│  │  ├─ entity.go
│  │  ├─ service.go
│  │  ├─ repository.go
│  │  ├─ events.go
│  │  └─ errors.go
│  ├─ <capability-b>/
│  │  ├─ entity.go
│  │  ├─ service.go
│  │  └─ repository.go
│  └─ platform/
│     ├─ clock.go
│     ├─ ids.go
│     └─ transactions.go
├─ go.mod
├─ go.sum
└─ Dockerfile
```

### What each area is for

#### `cmd/<service>`

Contains only the executable entrypoint.

Responsibilities:

- parse flags if needed
- load bootstrap config
- build app
- start app
- handle shutdown signals

Should not contain:

- business logic
- HTTP route implementation
- SQL queries
- MQTT message handling rules

`main.go` should stay small.

#### `internal/app`

Composition root for the service.

Responsibilities:

- load config
- create shared dependencies
- wire repositories, services, handlers, publishers, subscribers
- return a ready-to-run application object

Typical files:

- `config.go`: service-specific config struct and config loading helpers
- `wiring.go`: constructors and assembly
- `app.go`: application lifecycle, `Run()`, `Shutdown()`

This package owns dependency injection.

#### `internal/http`

HTTP transport only.

Responsibilities:

- routing
- request parsing
- validation of request shapes
- response shaping
- mapping transport errors to HTTP status codes
- HTTP middleware that is specific to the service

Should not contain:

- SQL
- domain decision-making
- direct infrastructure bootstrapping

Prefer handler files grouped by capability:

- `users_handler.go`
- `devices_handler.go`
- `installations_handler.go`

#### `internal/infra`

Concrete infrastructure adapters.

Examples:

- database repositories
- Redis-backed caches
- MQTT clients and publishers
- HTTP clients to other services
- file-system implementations
- third-party API adapters

This is where concrete code that talks to the outside world lives.

Typical subfolders:

- `infra/db`
- `infra/postgres` only when backend lock-in is intentional
- `infra/mqtt`
- `infra/redis`
- `infra/clients`

Prefer backend-neutral names like `db` when the service should be free to swap the concrete SQL backend later without a package rename.

#### `internal/<capability>`

Business capability packages.

These packages define:

- core entities or state types
- use-case services
- package-level errors
- repository or publisher interfaces used by the capability
- pure business rules

These packages should be the most stable part of a service.

---

## File naming rules

### Capability packages

Inside a capability package, use predictable file names.

Recommended:

- `entity.go` or `types.go`: core types for the capability
- `service.go`: business logic orchestration
- `repository.go`: repository interface only
- `publisher.go`: publisher interface only
- `subscriber.go`: subscriber interface only when needed
- `errors.go`: package-specific sentinel errors or typed errors
- `validation.go`: domain validation rules
- `events.go`: domain event payloads

Avoid vague names like:

- `server.go`
- `manager.go`
- `common.go`
- `helpers.go`
- `misc.go`

unless the package context makes the purpose obvious.

### HTTP package

Recommended:

- `router.go`
- `middleware.go`
- `users_handler.go`
- `devices_handler.go`
- `responses.go`
- `requests.go`

If a capability handler grows too large, split by use case:

- `users_get.go`
- `users_list.go`
- `users_update.go`

### Infrastructure packages

Name implementation files after the responsibility first, and after the backend only when that detail is intentionally part of the package contract:

- `repository.go`
- `sql_repository.go`
- `mqtt_publisher.go`
- `redis_cache.go`
- `marketplace_client.go`

If a package has only one implementation inside a focused package like `infra/db`, generic names like `repository.go` are preferred.

Use backend-specific file names only when the implementation detail must remain visible.

---

## Dependency injection rules

## Preferred pattern

Use constructor injection.

Example pattern:

```go
repo := db.NewRepository(sqlDB)
publisher := mqtt.NewDevicePublisher(client)
svc := users.NewService(repo, publisher, clock)
handler := http.NewUsersHandler(svc)
```

Rules:

- dependencies are passed into constructors
- structs keep their dependencies as fields
- no package should reach into unrelated global singletons
- wiring happens in one place only

## Where wiring belongs

Allowed places:

- `cmd/<service>/main.go`
- `internal/app/wiring.go`

Not allowed:

- inside handler constructors that secretly initialize databases
- inside business packages that create their own MQTT clients
- inside repository constructors that fetch global config from unrelated packages

## Interfaces and DI

Use interfaces where there is a real seam.

Good examples:

```go
type UserRepository interface {
    GetByID(ctx context.Context, id uuid.UUID) (User, error)
    Save(ctx context.Context, user User) error
}

type DevicePublisher interface {
    PublishCommand(ctx context.Context, cmd Command) error
}
```

Less good:

```go
type UserServiceInterface interface {
    DoEverything()
}
```

Use interfaces to decouple packages, not to satisfy a style rule.

## Optional compile-time checks

For concrete implementations, use compile-time interface assertions where useful:

```go
var _ users.UserRepository = (*Repository)(nil)
```

This is encouraged for infrastructure adapters.

---

## Package boundaries

### Business packages should not depend directly on transport packages

Do not let capability packages import `internal/http`.

### Business packages should not depend directly on concrete infrastructure packages unless the service is trivial

Prefer business packages to depend on small interfaces.
Concrete implementations belong in `internal/infra/...`.

### Transport packages may depend on business packages

This is expected.
Handlers call services.

### Infrastructure packages may depend on business packages

This is expected when implementing interfaces defined by the business package.

### `shared` packages are for stable cross-service infrastructure only

Good uses of `shared`:

- `envx`
- `dbx`
- `mqttx`
- `observability`
- `hdp`

Avoid putting service-specific business logic into `shared`.

If code is only used by one service, it stays in that service.

---

## DTO and model separation

One of the biggest maintainability problems is mixing multiple model types in the same package.

Separate these clearly:

### 1. Domain entities

These live in capability packages.
They represent business concepts.

Examples:

- `User`
- `Dashboard`
- `AutomationWorkflow`
- `InstalledIntegration`
- `DeviceBinding`

### 2. HTTP request/response DTOs

These live in `internal/http`.
They represent transport contracts.

Examples:

- `CreateUserRequest`
- `ListDevicesResponse`
- `UpdateIntegrationRequest`

### 3. Persistence models / row mapping

These live in `internal/infra/db` or the relevant infrastructure package.
They represent database storage concerns.

Examples:

- `userRow`
- `dashboardRecord`
- `automationWorkflowModel`

### 4. Messaging payloads

These live in the capability package or `internal/infra/mqtt` depending on ownership.

Examples:

- MQTT HDP command payloads
- internal event payloads
- adapter hello/status payloads

Rule:
Do not use one struct as the HTTP DTO, database row, and domain entity unless the service is truly tiny and the mapping is trivial.

---

## Recommended patterns by service type

Different Homenavi services are not identical. The structure should be consistent, but not rigid.

## A. Small utility / adapter service

Examples:

- `mock-adapter`
- `weather-service`
- smaller integration runtimes

Recommended structure:

```text
internal/
├─ app/
├─ http/
├─ infra/
│  ├─ mqtt/
│  └─ clients/
└─ adapter/ or forecast/ or sync/
```

These services may keep fewer packages, but should still separate:

- bootstrapping
- transport
- domain logic
- infrastructure

## B. CRUD/business service

Examples:

- `user-service`
- `dashboard-service`
- `auth-service`

Recommended structure:

```text
internal/
├─ app/
├─ http/
├─ infra/
│  ├─ db/
│  ├─ redis/
│  └─ clients/
├─ users/
├─ profiles/
├─ auth/
└─ sessions/
```

## C. Event-driven service

Examples:

- `history-service`
- `automation-service`
- `entity-registry-service`
- `device-hub`
- `zigbee-adapter`

Recommended structure:

```text
internal/
├─ app/
├─ http/
├─ infra/
│  ├─ mqtt/
│  ├─ db/
│  ├─ redis/
│  └─ clients/
├─ ingest/
├─ automation/
├─ devices/
├─ bindings/
└─ events/
```

These services often need separate files for:

- MQTT subscriptions
- message parsing
- state transitions
- persistence
- HTTP query endpoints

They should avoid one giant `server.go` that mixes all of them.

## D. Gateway / orchestration service

Examples:

- `api-gateway`
- `integration-proxy`

Recommended structure:

```text
internal/
├─ app/
├─ http/
├─ infra/
│  ├─ clients/
│  ├─ files/
│  └─ runtime/
├─ routes/
├─ authz/
├─ registry/
├─ installations/
└─ updates/
```

These services often accumulate too much logic in routing packages.
The fix is to split by orchestrated capability, not by transport only.

---

## Guidance for current Homenavi problem areas

### Giant `server.go` files

If a file named `server.go` is hundreds of lines long, it should probably be split into:

- `router.go`
- `install_handler.go`
- `update_handler.go`
- `registry_handler.go`
- `proxy_handler.go`
- `status_handler.go`
- `response.go`
- `request.go`

If domain logic is embedded inside handlers, move it into a capability package service.

### Mixed structs and logic

If a file contains:

- request structs
- response structs
- repository interfaces
- service methods
- validation
- message publishing

split it by responsibility first, then by size.

### Large transport packages

If `internal/httpapi` or similar becomes a dumping ground, replace it with:

- `internal/http`
- `internal/<capability>`
- `internal/infra/<backend>`

### Overloaded `config` package

Keep config loading in `internal/app/config.go` or `internal/platform/config.go`.
Avoid letting every package read environment variables directly.

Rule:
- read env in one place
- pass typed config down through constructors

### Adapters that both subscribe and run business rules in one file

Split into:

- `subscriber.go`: MQTT binding and subscription setup
- `translator.go`: protocol payload → internal/domain data
- `service.go`: orchestration and rule execution
- `publisher.go`: outgoing MQTT/HDP publication

---

## Testing conventions

## File placement

Tests should live beside the package they test.

Examples:

- `service_test.go`
- `repository_test.go`
- `handler_test.go`
- `mqtt_publisher_test.go`

## What to test where

### Capability package tests

Test:

- business rules
- validation
- orchestration logic
- error handling

Prefer fakes/mocks over real DB/MQTT where practical.

### HTTP tests

Test:

- request decoding
- response codes
- routing
- auth/middleware behavior
- DTO mapping

### Infrastructure tests

Test:

- SQL mapping
- MQTT publish behavior
- external client request building
- serialization/deserialization

## Fakes and test doubles

Prefer simple hand-written fakes for narrow interfaces.
Avoid elaborate mock frameworks unless justified.

Good:

- fake repository
- fake publisher
- fake clock
- fake external API client

---

## Naming conventions

### Package names

Use short lowercase package names.
Avoid stutter where possible.

Good:

- `users`
- `devices`
- `installations`
- `bindings`
- `forecast`

Avoid:

- `usermodels`
- `userservice`
- `devicehandler`

### Constructor names

Use `New...` consistently.

Examples:

- `NewService`
- `NewRepository`
- `NewHandler`
- `NewClient`
- `NewApp`

### Interface names

Do not automatically suffix every interface with `Interface`.

Good:

- `Repository`
- `Publisher`
- `Clock`
- `Runtime`
- `CatalogClient`

Only use more specific names when the package context requires it.

### Implementation names

Concrete implementations may be generic inside their package if the package scope is already specific.

Examples:

- package `postgres`: `type Repository struct`
- package `mqtt`: `type Publisher struct`

If needed for clarity, use explicit names like `UserRepository`.

---

## When to split a package

Split a package when:

- it owns more than one unrelated concept
- files repeatedly cross domain concerns
- testing requires too many unrelated dependencies
- package imports become tangled
- package name no longer clearly describes what it does

Do not split a package merely because it has multiple files.
A capability package with several focused files is healthy.

---

## When not to over-engineer

These conventions are not a license to create excessive abstraction.

Avoid:

- introducing interfaces with only one expected implementation and no boundary value
- creating deep nested package trees for tiny services
- duplicating trivial mapping layers everywhere without benefit
- turning a small service into a full clean-architecture exercise prematurely

The target is pragmatic consistency, not theoretical purity.

Rule of thumb:

- small services may be simpler
- all services should still respect the same separation ideas
- complexity should be added only where it improves maintainability

---

## Recommended default templates

## Template: small service

```text
internal/
├─ app/
│  ├─ app.go
│  └─ config.go
├─ http/
│  ├─ router.go
│  └─ handler.go
├─ infra/
│  └─ clients/
└─ <capability>/
   ├─ types.go
   └─ service.go
```

## Template: medium service

```text
internal/
├─ app/
│  ├─ app.go
│  ├─ config.go
│  └─ wiring.go
├─ http/
│  ├─ router.go
│  ├─ middleware.go
│  ├─ <capability-a>_handler.go
│  └─ <capability-b>_handler.go
├─ infra/
│  ├─ db/
│  ├─ redis/
│  └─ clients/
├─ <capability-a>/
│  ├─ entity.go
│  ├─ service.go
│  └─ repository.go
└─ <capability-b>/
   ├─ entity.go
   ├─ service.go
   └─ repository.go
```

## Template: event-driven service

```text
internal/
├─ app/
├─ http/
├─ infra/
│  ├─ mqtt/
│  ├─ db/
│  ├─ redis/
│  └─ clients/
├─ ingest/
│  ├─ service.go
│  ├─ parser.go
│  └─ subscriber.go
├─ devices/
│  ├─ entity.go
│  ├─ service.go
│  └─ repository.go
└─ events/
   ├─ publisher.go
   ├─ types.go
   └─ mapper.go
```

---

## Migration guidance for existing services

This document does not require rewriting every service at once.
Migration should be incremental.

### Recommended order for refactors

1. Extract composition root
   - move startup and wiring to `internal/app`
2. Split giant HTTP files
   - separate routes, handlers, DTOs, and response helpers
3. Extract capability packages
   - move business logic out of transport files
4. Move concrete DB/MQTT/http implementations into `internal/infra`
5. Add constructor-based DI where globals or implicit dependencies still exist
6. Split oversized files until each has one clear purpose

### Safe first refactor targets

Best early wins:

- giant `server.go` files
- mixed config/env access spread across multiple packages
- packages that directly initialize DB/MQTT inside handlers
- files containing both transport DTOs and business rules

---

## Homenavi-specific service guidance

### `api-gateway`

Prefer capability split by routing concern:

- `routes`
- `proxy`
- `authz`
- `websocket`
- `ratelimit`

Keep transport and proxy mechanics separate from route config loading.

### `auth-service`

Likely capabilities:

- `auth`
- `sessions`
- `oauth`
- `verification`
- `passwordreset`
- `twofactor`

Avoid one giant handler area for unrelated auth flows.

### `automation-service`

Likely capabilities:

- `workflows`
- `runs`
- `triggers`
- `actions`
- `subscriptions`

MQTT subscription handling should not live in the same file as rule execution if it becomes large.

### `dashboard-service`

Likely capabilities:

- `dashboards`
- `layouts`
- `widgets`

Persistence and JSON document shaping should be clearly separated.

### `device-hub`

Likely capabilities:

- `devices`
- `pairing`
- `adapters`
- `commands`
- `registry`

HDP/MQTT message handling should be split from HTTP-facing orchestration.

### `entity-registry-service`

Likely capabilities:

- `rooms`
- `tags`
- `devices`
- `bindings`
- `autoimport`
- `realtime`

Realtime/websocket concerns should not be mixed deeply into CRUD handler logic.

### `history-service`

Likely capabilities:

- `ingest`
- `query`
- `retention`

This service should remain simple, but still keep ingestion separate from query API.

### `integration-proxy`

Likely capabilities:

- `registry`
- `installations`
- `updates`
- `runtime`
- `proxying`
- `artifacts`

This is a strong candidate for splitting a large server package by capability.

### `mock-adapter` and `zigbee-adapter`

Likely capabilities:

- `adapter`
- `pairing`
- `commands`
- `state`
- `discovery`

Protocol parsing, MQTT transport, and HDP publication should not all sit in one file as the implementation grows.

### `user-service`

Likely capabilities:

- `users`
- `profiles`
- `admin`
- `search`

### `weather-service`

Likely capabilities:

- `forecast`
- `cache`
- `providers`

Simple service, but still benefit from separate provider client and HTTP transport.

---

## Repository-wide rules summary

All Homenavi Go services should follow these rules:

1. Use `cmd/<service>` for entrypoints only.
2. Use constructor-based manual dependency injection.
3. Keep wiring in `internal/app`.
4. Separate transport, business logic, and infrastructure.
5. Organize business code by capability/domain concept.
6. Avoid global `models`, `services`, and `helpers` dumping-ground packages.
7. Keep files small and responsibility-focused.
8. Split any file that mixes multiple unrelated concerns.
9. Keep HTTP DTOs, domain entities, persistence models, and messaging payloads separate.
10. Use interfaces only at real seams.
11. Keep `shared` focused on reusable infrastructure and protocols, not service business logic.
12. Prefer consistency across services over local one-off layouts.

---

## Decision statement

For Homenavi, the standard architecture for Go services is:

- package-by-capability
- manual constructor-based dependency injection
- thin entrypoints
- dedicated composition root
- transport separated from business logic
- infrastructure adapters separated from business logic
- small focused files with consistent naming

This is the default structure going forward.
Any service that diverges should do so only intentionally and with a clear reason.
