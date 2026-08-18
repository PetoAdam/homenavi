# Homenavi MQTT Shared Subscriptions and HA Plan

## Goal
Make the event-driven parts of Homenavi horizontally scalable by using EMQX shared subscriptions and stateless consumers, so services can run with multiple replicas without duplicate message processing.

Secondary goal:
- make the shared MQTT client layer broker-agnostic so shared-subscription support is implemented as a broker capability and strategy, not hard-coded into service code or the public client API.

Enterprise-grade success criteria:
- preserve correctness before optimizing scalability
- make delivery semantics explicit at every subscription point
- keep the public MQTT abstraction clean and maintainable as the platform evolves toward `v1.0`
- make rollout observable and safe under partial deployment
- require documented ownership and verification for every shared-subscription decision

## Enterprise design principles

This plan should be executed under the following engineering rules.

### 1. Correctness over throughput
- no topic moves to shared mode until the handler's idempotency, ordering tolerance, and side effects are understood
- single-replica correctness remains the baseline contract

### 2. Explicit contracts over convention
- delivery mode must be encoded in subscription options, not inferred from topic strings or deployment shape
- broker capabilities must be declared and validated explicitly

### 3. Clear migration path
- existing exclusive consumers must keep working throughout the migration until they are intentionally moved
- because the platform is still pre-`v1.0`, the new `shared/mqttx` API can replace the old one directly instead of carrying a deprecation layer

### 4. Operational visibility is mandatory
- connect, disconnect, resubscribe, message-consume, handler-error, replay, and dedupe outcomes must be visible in logs and metrics
- rollout is blocked if operators cannot tell whether shared-mode behavior is healthy

### 5. Small trusted surface
- broker-specific behavior belongs in one strategy layer
- client-library-specific behavior belongs in one transport adapter layer
- service code should only describe intent and domain behavior

## Non-goals

This plan does not require:
- immediate migration of every integration repository to the shared helper
- forcing specialized external transport clients into the same abstraction if it weakens correctness
- solving exactly-once delivery at the broker level
- removing all broker-specific behavior from the system; the goal is to isolate it cleanly

## Current state
The repo currently uses direct MQTT subscriptions in consumers such as:
- `device-hub`
- `automation-service`
- `entity-registry-service`
- `history-service`
- `mock-adapter`
- `zigbee-adapter`

Implemented foundation:
- `shared/mqttx` now has typed subscription options, broker-kind selection, and broker topic strategies for EMQX shared subscriptions and generic MQTT.
- `history-service` and `entity-registry-service` can opt into shared consumer groups through service configuration.
- Core services now pass typed MQTT configuration through their wrappers.
- The shared MQTT callback boundary exposes a neutral message/handler contract; Paho remains inside `shared/mqttx` as the transport implementation.

Remaining migration work:
- classify and externalize state ownership before moving higher-risk `automation-service` or `device-hub` topic families to shared mode.
- decide whether and how adjacent integration repositories should adopt the common client abstraction.

Device-hub migration status:
- adapter presence is stored in Redis with a bounded freshness TTL; adapter hello and status delivery can use `DEVICE_HUB_MQTT_SHARED_GROUP`.
- metadata upserts and device-removal events can use the same shared group because Postgres is authoritative and their handlers are idempotent.
- device state, command results, and pairing progress remain exclusive. Pending command correlation, timeout ownership, and pairing-session transitions must move to Redis/Postgres before those topics can share delivery.

The remaining architectural work is now narrower:
- Paho is isolated to `shared/mqttx` as the active transport implementation.
- Service-local wrappers expose their own message contracts while delegating connection, replay, and broker strategy behavior to `shared/mqttx`.
- A future transport replacement should only need a new implementation behind the shared client boundary, not service-handler rewrites.

## Workspace audit of MQTT client setup

This section checks the current code paths where an MQTT client is constructed and classifies whether the shared-subscription and broker-agnostic design should apply.

### Core repo clients built on `shared/mqttx`

#### `shared/mqttx`
- File: `shared/mqttx/mqtt.go`
- Role: common MQTT facade with Paho as the current internal transport implementation
- Current behavior: URL normalization, reconnect policy, TLS handling, subscription replay, and broker-aware subscription resolution
- Fit for broker-agnostic plan: yes, this is the primary implementation target
- Fit for shared subscriptions: yes, this is where shared-subscription intent and broker strategy should live

#### `automation-service/internal/infra/mqtt/client.go`
- Role: service-local wrapper over `shared/mqttx`
- Current behavior: QoS 1 subscribe and publish, `OnConnect` fan-out for workflow refresh behavior
- Fit for broker-agnostic plan: yes
- Fit for shared subscriptions: yes, selectively for state and command-result consumers after idempotency review

#### `device-hub/internal/infra/mqtt/client.go`
- Role: service-local wrapper over `shared/mqttx`
- Current behavior: local message/handler contract and publish with optional retain
- Fit for broker-agnostic plan: yes
- Fit for shared subscriptions: partially; some event ingestion can move to shared mode, but not every topic should

#### `entity-registry-service/internal/infra/mqtt/client.go`
- Role: service-local wrapper over `shared/mqttx`
- Current behavior: simple subscribe-only consumer wrapper used by autoimport
- Fit for broker-agnostic plan: yes
- Fit for shared subscriptions: yes, strong early candidate

#### `history-service/internal/infra/mqtt/client.go`
- Role: service-local wrapper over `shared/mqttx`
- Current behavior: QoS 1 subscribe wrapper for append-oriented ingest
- Fit for broker-agnostic plan: yes
- Fit for shared subscriptions: yes, best first-wave candidate

#### `zigbee-adapter/internal/infra/mqtt/client.go`
- Role: service-local wrapper over `shared/mqttx`
- Current behavior: persistent session, `CleanSession: false`, `ResumeSubs: true`
- Fit for broker-agnostic plan: yes
- Fit for shared subscriptions: mixed; the adapter has both broker-facing bridge duties and command consumption, so it should not be blindly converted

#### `mock-adapter/internal/app/app.go`
- Role: direct app construction over `shared/mqttx`
- Current behavior: simple singleton adapter setup
- Fit for broker-agnostic plan: yes
- Fit for shared subscriptions: maybe later, but only if the adapter is intentionally turned into a distributed consumer role

### Integration repos with direct Paho setup

These are outside the core `shared/mqttx` package today, but they still matter for the overall design review.

#### `homenavi-connector/src/backend/cmd/integration/main.go`
- Role: direct Paho client for connector command consumption and status publishing
- Current behavior: reconnect enabled, subscribe-on-connect, publishes hello/status on reconnect
- Fit for broker-agnostic plan: yes, as a future consumer of a reusable neutral client package or a copied adapter pattern
- Fit for shared subscriptions: limited; command handling could be shareable only if one logical connector workload is meant to be horizontally scaled

#### `homenavi-emodul/src/backend/bridge.go`
- Role: direct Paho singleton device bridge
- Current behavior: reconnect enabled, subscribe-on-connect, active sync loop, publishes hello/status and sync-triggered state
- Fit for broker-agnostic plan: yes, but likely as an exclusive singleton client profile rather than as a shared-consumer participant
- Fit for shared subscriptions: generally no for the current bridge topology; this is primarily a singleton adapter role

#### `homenavi-lg-thinq/src/backend/cmd/integration/main.go`
- Role: direct Paho client for local Homenavi broker interaction
- Current behavior: explicit TLS/user parsing, one-off connection helper with retry wrapper
- Fit for broker-agnostic plan: yes, for the local Homenavi-broker-facing client
- Fit for shared subscriptions: possibly for Homenavi command consumers only, not by default

#### `homenavi-lg-thinq/src/backend/cmd/integration/realtime_bridge.go`
- Role: direct Paho client to an external LG realtime endpoint
- Current behavior: custom TLS config, websocket subprotocol headers, custom dialer, `SetDefaultPublishHandler`, `AutoReconnect(false)`
- Fit for broker-agnostic plan: only partially
- Fit for shared subscriptions: no
- Important note: this client is not a Homenavi broker consumer. It should be treated as a specialized external realtime transport and may remain outside the generic shared-subscription path.

## Applicability decision

The shared-subscription idea works across the Homenavi core consumers, but it should not be forced onto every MQTT client in the workspace.

Safe applicability:
- `history-service`
- `entity-registry-service`
- selected subscriptions in `automation-service`
- selected subscriptions in `device-hub`

Conditional applicability:
- `zigbee-adapter`
- `mock-adapter`
- `homenavi-connector`
- local Homenavi-broker client paths in `homenavi-lg-thinq`

Out of shared-subscription scope, but still relevant to broker-agnostic client design:
- `homenavi-emodul` singleton bridge
- `homenavi-lg-thinq` external realtime bridge

That means the design should support three client profiles:
- shared-consumer profile for horizontally scaled Homenavi consumers
- exclusive singleton profile for adapter/bridge processes
- specialized transport profile for unusual external broker connections that need extra TLS, websocket, or protocol options

## Review of the current plan

What is already strong:
- The plan correctly treats shared subscriptions as selective, not universal.
- The rollout order is sensible: start with idempotent consumers such as `history-service` or `entity-registry-service` before moving to more stateful paths.
- The statelessness and replay-safety requirements are correctly called out.

What is implemented or should be tightened:
- EMQX shared-subscription syntax is isolated behind a broker topic strategy rather than exposed to service code.
- `shared/mqttx/paho_transport.go` now keeps Paho connection and operation details behind an internal transport interface; the public client uses a neutral message and handler contract.
- Broker-agnostic refactoring and EMQX shared-mode rollout remain separate: the former is complete for core services, while the latter is enabled only for the safe first-wave consumers.
- Capability checks reject shared mode on generic brokers.
- Topic rewriting and subscription intent are explicit through `SubscriptionOptions`; callers do not construct `$share/<group>/...` strings.

## Design rule for the client layer

The public contract in `shared/mqttx` should describe messaging behavior, not Paho behavior and not EMQX syntax.

Specifically:
- service code should depend on broker-neutral interfaces
- broker-specific topic rewriting should live behind a strategy or adapter
- client-library-specific code should stay inside an implementation package
- shared subscriptions should be expressed as subscription intent, not as hard-coded topic strings
- unsupported broker capabilities should fail explicitly at connect or subscribe time

Additional enterprise rule:
- public interfaces should stay small and intentional so they remain easy to evolve before `v1.0`

## Target architecture for broker-agnostic MQTT

Use a ports-and-adapters design inside `shared/mqttx`.

Recommended patterns:
- Adapter: wrap Paho or any future client library behind a small transport interface
- Strategy: resolve subscription mode into the correct broker-specific topic form
- Factory: choose the concrete transport and broker capability set from config
- Decorator: attach logging, metrics, tracing, and reconnect behavior without changing service code

Supporting enterprise patterns:
- Facade: present a small stable API to services while internal transport and strategy pieces evolve
- Policy object: store delivery-mode and capability decisions in typed configuration rather than booleans scattered through code

### 1. Broker-neutral public model

Move the shared package toward a neutral API surface such as:

- `type Message interface { Topic() string; Payload() []byte; QoS() byte; Retained() bool; Duplicate() bool }`
- `type Handler func(Message)`
- `type PublishOptions struct { QoS byte; Retain bool }`
- `type SubscriptionMode string`
- `const ( SubscriptionModeExclusive SubscriptionMode = "exclusive"; SubscriptionModeShared SubscriptionMode = "shared" )`
- `type SubscriptionOptions struct { Topic string; QoS byte; Mode SubscriptionMode; Group string }`
- `type Client interface { Subscribe(SubscriptionOptions, Handler) error; Publish(topic string, payload []byte, opts PublishOptions) error; Unsubscribe(topic string) error; Close() error; IsConnected() bool }`

Important constraint:
- this public contract should not alias third-party types from `github.com/eclipse/paho.mqtt.golang`

### 2. Broker capability model

Add an explicit broker capability layer so the caller can request behavior without knowing syntax details.

Recommended shape:
- `type Capabilities struct { SharedSubscriptions bool; PersistentSessions bool; WildcardSubscriptions bool }`
- `type TopicStrategy interface { ResolveSubscribeTopic(SubscriptionOptions) (string, error); Capabilities() Capabilities }`

Examples:
- `EMQXTopicStrategy` supports shared subscriptions via `$share/<group>/<topic>`
- `GenericMQTTTopicStrategy` supports only standard MQTT topics and rejects `SubscriptionModeShared`

That gives a clean rule:
- service code requests shared semantics
- strategy decides whether and how that maps to broker syntax

### 3. Transport adapter boundary

Split transport concerns from topic-strategy concerns.

Recommended internal seam:
- `type transportClient interface { Subscribe(topic string, qos byte, handler Handler) error; Publish(topic string, payload []byte, opts PublishOptions) error; Unsubscribe(topic string) error; Close() error; IsConnected() bool }`

Then implement:
- `pahoTransport` in `shared/mqttx/internal/paho` or `shared/mqttx/pahoimpl`

Responsibilities of the transport adapter:
- connect and reconnect through the chosen client library
- adapt library-native messages to the broker-neutral `Message` interface
- preserve subscription replay on reconnect
- stay unaware of EMQX `$share/...` policy beyond receiving a fully resolved topic

### 4. Composite client in shared/mqttx

The exported client should compose:
- transport adapter
- topic strategy
- remembered subscription registry for reconnect replay

Flow:
1. service calls `Subscribe(SubscriptionOptions{Mode: SubscriptionModeShared, Group: "history-ingest", Topic: ...}, handler)`
2. shared client asks the topic strategy to resolve the actual subscribe topic
3. resolved topic is passed to the transport adapter
4. reconnect replay uses the original subscription options, not raw rewritten topics

This matters because it keeps shared-subscription policy reversible and testable.

## Configuration model

Introduce explicit config for both transport and broker capabilities.

Recommended additions:
- `MQTT_BROKER_URL`
- `MQTT_CLIENT_ID`
- `MQTT_CLIENT_ID_PREFIX`
- `MQTT_DRIVER=paho`
- `MQTT_BROKER_KIND=emqx|generic`
- `MQTT_SHARED_SUBSCRIPTIONS_ENABLED=true|false`
- `MQTT_SHARED_SUBSCRIPTION_PREFIX` only if a broker variant needs a non-default mapping later
- `MQTT_PROFILE=shared-consumer|exclusive-singleton|specialized`

Rules:
- `MQTT_DRIVER` chooses the client-library adapter
- `MQTT_BROKER_KIND` chooses the topic strategy/capabilities
- `MQTT_SHARED_SUBSCRIPTIONS_ENABLED` is a deployment-level guardrail, not a substitute for capability checks
- `MQTT_PROFILE` chooses safe defaults for session and reconnect behavior, while still allowing explicit overrides where needed

Do not overload the broker URL to imply broker features.

Additional requirement from the workspace audit:
- keep room for advanced transport options on specialized clients such as websocket headers, protocol version, custom dialer, or TLS server-name overrides

Recommended approach:
- keep the common API small
- allow transport-specific options through an internal adapter config or advanced options struct
- do not leak those advanced knobs into normal service code unless the service truly needs them

## Observability, SLOs, and operability

The migration is not enterprise-grade unless operators can detect degraded behavior quickly.

Required telemetry from the shared MQTT layer:
- connection established count
- connection lost count with reason classification
- reconnect attempts and success latency
- active subscription count
- resubscribe success/failure count
- messages received by topic family and subscription mode
- handler success/failure count
- dedupe suppressions, where dedupe exists
- shared-subscription capability rejection count

Recommended dimensions:
- service name
- broker kind
- client profile
- subscription mode
- consumer group
- topic family

Operational expectations:
- logs must include enough structured context to correlate reconnect storms and message-processing anomalies
- dashboards should separate transport failures from handler failures

## Security and resilience guidance

This is a pre-`v1.0` smart-home platform, so the MQTT design should be safe and reasonable without turning local integration development into a burden.

Practical guidance:
- preserve current TLS behavior where it already exists, but do not require TLS for every local or homelab MQTT integration path
- support username/password or anonymous/local broker access depending on deployment needs
- never log raw credentials or secret values
- keep advanced transport options opt-in and scoped to the clients that need them
- prefer deterministic client IDs and shared group names for debuggability, but do not over-design this before `v1.0`

Resilience rules:
- treat duplicate delivery, reconnect storms, and partial broker outages as first-class test scenarios
- do not assume retained bootstrap plus reconnect replay always arrives in a convenient order
- make handler-level timeouts, cancellation, and shutdown behavior explicit where the service runtime requires them

## Subscription classification standard

Before any topic is migrated, the owning service must classify each subscription with a short decision record.

For each subscription record:
- topic or topic family
- current consumer service
- delivery mode decision: shared, exclusive, deferred
- reason for that decision
- idempotency strategy
- ordering assumption
- retained-message expectation
- tests covering the decision

This is required because enterprise-grade messaging changes fail most often when semantic decisions stay implicit.

### Current core subscription decisions

| Service | Topic family | Decision | Reason |
|---|---|---|---|
| `history-service` | HDP state ingest | Shared when `HISTORY_SERVICE_MQTT_SHARED_GROUP` is set | Append-oriented ingest is the first shared-consumer pilot. |
| `entity-registry-service` | HDP metadata, state, and events | Shared when `ENTITY_REGISTRY_MQTT_SHARED_GROUP` is set | Canonical repository writes tolerate replay and own the durable result. |
| `automation-service` | HDP state and command results | Shared when `AUTOMATION_SERVICE_MQTT_SHARED_GROUP` is set | Postgres atomically claims state and schedule cooldowns; Redis distributes run-event fan-out and replay across replicas; pending command correlations already persist in Postgres. |
| `device-hub` | Adapter status, metadata, state, events, command results, pairing progress | Shared when `DEVICE_HUB_MQTT_SHARED_GROUP` is set | Adapter availability is Redis-backed; device state and lifecycle transitions are owned by Postgres, with Redis only coordinating active commands. |

### Statelessness Completion Plan

#### Automation service

Implemented:
- Workflow definitions are loaded from Postgres and may be cached locally because they are derived data.
- `trigger_cooldowns` in Postgres atomically claims both state-trigger and schedule-trigger cooldown windows.
- `pending_correlations` in Postgres already provides durable command-result ownership.
- Redis stores a bounded replay list that WebSocket replicas poll, so clients can attach to any replica without Redis Pub/Sub.
- When `AUTOMATION_SERVICE_MQTT_SHARED_GROUP` is configured on EMQX, state and command-result topics use one shared consumer group.

Remaining follow-up:
- Add a distributed schedule leader or dedicated schedule work queue if large numbers of schedule triggers make every replica's cron reconciliation expensive. The durable cooldown claim already prevents duplicate runs.
- Move selector cache invalidation onto an ERS-driven notification path if cache freshness becomes a multi-replica concern.

#### Device hub

Postgres should own durable state:
- device metadata and normalized device state
- command history and terminal command outcomes
- pairing session history and completed/failed session outcomes

Redis should own short-lived shared state:
- adapter availability and capability snapshots with TTL
- active pairing sessions and expiry timers
- pending command correlation-to-device mappings and TTLs
- per-device command ownership locks
- device-hub realtime invalidation and fan-out channels

Migration sequence:
1. Extract `adapterRegistry` behind a Redis-backed snapshot store; retain in-memory data only as a local cache.
2. Persist pairing session transitions in Postgres and mirror active sessions into Redis with their expiry.
3. Move pending command lifecycle maps to Redis hashes/keys with TTL and use a per-device ownership lock during command dispatch.
4. Publish device-hub invalidation events through Redis so every HTTP/WebSocket replica refreshes its local cache.
5. Move state/metadata/event ingestion to an EMQX shared group after the relevant Redis/Postgres state is authoritative.
6. Move command-result and pairing-progress topics only after command and pairing ownership are fully shared.

## Shared subscription API design

Do not make service code hand-build shared topic strings.

Prefer one of these shapes:

### Option A: single explicit subscribe method
- `Subscribe(opts SubscriptionOptions, handler Handler) error`

This is the most future-proof option because subscription behavior is represented in one place.

### Option B: convenience helpers on top of the same model
- `SubscribeTopic(topic string, qos byte, handler Handler) error`
- `SubscribeShared(group, topic string, qos byte, handler Handler) error`

If convenience helpers are added, they should delegate to the same `SubscriptionOptions` path. They should not bypass strategy resolution.

## Service wrapper design

Keep service-local wrappers, but narrow them to domain needs and make them depend on the neutral `mqttx` interfaces.

Recommended direction:
- `device-hub/internal/infra/mqtt` should stop aliasing `mqttx.Handler` and `mqttx.Message`
- `automation-service/internal/infra/mqtt` and `history-service/internal/infra/mqtt` should wrap the neutral message interface rather than embedding `mqttx.Message`
- `entity-registry-service/internal/infra/mqtt` should subscribe through subscription options so autoimport can be switched to shared mode without raw topic rewrites in service code
- adapter services should keep simple publish/subscribe wrappers, but their interfaces should remain library-neutral as well

This keeps the service boundary stable if Homenavi later uses a different client library or needs broker-specific fallbacks.

Additional rule from the audit:
- do not require integration repos or external-bridge code to adopt shared subscriptions just because they use MQTT; first classify whether they are core event consumers, singleton adapters, or specialized third-party realtime clients

## Statelessness requirements
To make replica counts safe, handlers need to be replay-safe:
- No in-memory authoritative state for consumer progress.
- Durable writes should be idempotent.
- Any dedupe should use message IDs, correlation IDs, or stable entity IDs.
- Cache warmers or sync state should live in Redis or Postgres, not process memory.

## Ordering and idempotency
MQTT shared subscriptions deliver each message to one consumer in the group, but you still need to think about ordering.

Plan for:
- Per-entity ordering where the topic layout naturally scopes the stream.
- Idempotent writes if the same event can be retried.
- No assumption that two related messages land on the same pod unless the topic design makes that likely.

Additional rule:
- if a workflow truly requires total in-order handling for a topic family, do not move it to shared mode until ordering is either externalized or narrowed to an entity-specific stream.

## Recommended file-by-file implementation plan

### Phase -1: client inventory and contract freeze

Files to inspect and record before code changes:
- `shared/mqttx/mqtt.go`
- `automation-service/internal/infra/mqtt/client.go`
- `device-hub/internal/infra/mqtt/client.go`
- `entity-registry-service/internal/infra/mqtt/client.go`
- `history-service/internal/infra/mqtt/client.go`
- `zigbee-adapter/internal/infra/mqtt/client.go`
- `mock-adapter/internal/app/app.go`
- `homenavi-connector/src/backend/cmd/integration/main.go`
- `homenavi-emodul/src/backend/bridge.go`
- `homenavi-lg-thinq/src/backend/cmd/integration/main.go`
- `homenavi-lg-thinq/src/backend/cmd/integration/realtime_bridge.go`

Tasks:
- classify each client as shared-consumer, exclusive singleton, or specialized external transport
- record required features per client: reconnect, resubscribe, retained delivery, QoS, clean session, resume subscriptions, TLS, websocket headers, custom dialer, explicit publish handler
- freeze the initial neutral API shape so migration does not oscillate while services are being converted
- create the first subscription-classification matrix for `history-service` and `entity-registry-service`

Acceptance criteria:
- every known MQTT client setup in the workspace is mapped to one of the supported client profiles
- no implementation work starts before unsupported edge cases are identified
- the target API and first subscription classifications are documented and reviewable

### Phase 0: split the abstraction boundary before adding broker-specific behavior

Files to change first:
- `shared/mqttx/mqtt.go`
- `shared/mqttx/config.go`
- `shared/mqttx/mqtt_test.go`
- new neutral types file such as `shared/mqttx/types.go`
- new strategy file such as `shared/mqttx/topic_strategy.go`
- new transport adapter file such as `shared/mqttx/paho_transport.go`

Tasks:
- replace exported Paho type aliases with broker-neutral interfaces and handler types
- keep the existing reconnect and replay behavior, but store original subscription options instead of only raw topics
- add capability-aware subscription resolution
- replace the old `mqttx` surface with the new one directly and update callers in the same migration
- add an advanced transport option path for rare clients that need websocket headers, TLS server-name overrides, or protocol-version knobs
- add structured logging and metric hooks at the shared client boundary

Acceptance criteria:
- no service has to import Paho types directly
- `shared/mqttx` can reject unsupported shared-subscription requests explicitly
- existing exclusive subscriptions keep working unchanged
- specialized external clients are either representable through advanced transport options or explicitly left outside the common path by design
- transport and capability failures are distinguishable through metrics and logs

### Phase 1: add broker strategies

Add at least:
- `EMQXTopicStrategy`
- `GenericMQTTTopicStrategy`

Tasks:
- map shared subscriptions to `$share/<group>/<topic>` only in the EMQX strategy
- reject shared subscriptions in the generic strategy with a clear error
- add config-driven factory selection for broker kind
- define deterministic validation for group naming, topic input, and invalid option combinations

Acceptance criteria:
- shared-subscription topic syntax exists in one place only
- unit tests cover EMQX and generic broker behavior separately
- invalid delivery-mode requests fail early with actionable errors

### Phase 2: add public shared-subscription helpers on top of the neutral model

Files:
- `shared/mqttx/mqtt.go`
- `shared/mqttx/mqtt_test.go`

Tasks:
- add `SubscribeShared(...)` convenience helpers if desired
- ensure helpers delegate to the same `SubscriptionOptions` path used by the generic `Subscribe(...)`
- ensure reconnect replay preserves mode/group/topic correctly

Acceptance criteria:
- no service constructs `$share/...` directly
- reconnect replay resubscribes using original subscription intent

### Phase 3: migrate service-local wrappers to neutral contracts

Files likely involved:
- `device-hub/internal/infra/mqtt/client.go`
- `automation-service/internal/infra/mqtt/client.go`
- `entity-registry-service/internal/infra/mqtt/client.go`
- `history-service/internal/infra/mqtt/client.go`
- `zigbee-adapter/internal/infra/mqtt/client.go`
- `mock-adapter/internal/adapter/service.go`
- affected tests in each service

Tasks:
- remove direct aliasing of `mqttx.Message` and `mqttx.Handler`
- convert wrappers to their own message/handler interfaces or plain function types over the neutral `mqttx.Message`
- add interface seams where tests currently depend on the concrete `*mqttx.Client`
- add wrapper-level observability labels or service identity where needed for telemetry

Acceptance criteria:
- service packages compile without relying on Paho-shaped types from `mqttx`
- unit tests can use simple fakes without importing broker-library details
- service-level logs and metrics remain attributable after the abstraction change

### Phase 3a: convert core singleton adapters to the neutral client without shared-mode rollout

Files likely involved:
- `zigbee-adapter/internal/infra/mqtt/client.go`
- `mock-adapter/internal/app/app.go`
- `mock-adapter/internal/adapter/service.go`

Tasks:
- migrate them to the neutral client contract
- preserve persistent-session behavior where currently required
- keep all subscriptions exclusive unless a later topology decision says otherwise

Acceptance criteria:
- adapter behavior is unchanged after the neutral-client migration
- shared-subscription rollout remains opt-in and separate

### Phase 3b: decide integration-repo reuse strategy

Repos and files:
- `homenavi-connector/src/backend/cmd/integration/main.go`
- `homenavi-emodul/src/backend/bridge.go`
- `homenavi-lg-thinq/src/backend/cmd/integration/main.go`
- `homenavi-lg-thinq/src/backend/cmd/integration/realtime_bridge.go`

Tasks:
- decide whether these repos should import a reusable broker-neutral MQTT helper package from the core repo, copy the same pattern locally, or intentionally stay direct-Paho for now
- explicitly exclude the LG ThinQ external realtime bridge from shared-subscription rollout
- if reuse is chosen, define the minimal public package boundary needed for these repos

Acceptance criteria:
- no integration repo is accidentally broken by assumptions that only hold for core Homenavi services
- the reuse story is explicit before attempting cross-repo refactors

### Phase 4: convert first shared consumers

Recommended first wave:
- `history-service`
- `entity-registry-service`

Why first:
- these are easier to make replay-safe than device command orchestration or workflow execution

Tasks:
- identify subscription call sites and convert selected consumers to `SubscriptionModeShared`
- assign explicit group names such as `history-ingest` and `entity-registry-autoimport`
- validate duplicate-side-effect resistance under 2+ replicas

Acceptance criteria:
- repeated message delivery does not create incorrect duplicate writes or imports
- horizontal scaling of those consumers works under pod churn
- the migration does not require any changes to singleton adapters or external realtime bridges

### Phase 5: convert higher-risk consumers

Second wave:
- `automation-service`
- `device-hub`

Tasks:
- `automation-service`: persist cooldown claims in Postgres, publish/replay run events through Redis, and enable one shared group for state and command-result topics.
- `device-hub`: classify topic families by durable state, Redis transient state, and local-only derived caches before enabling any shared group.
- Move correlation-ID or entity-ID-based dedupe into Postgres or Redis where required.
- Keep device-hub command-result and pairing-progress topics exclusive until shared command and pairing ownership are safe.
- Explicitly identify topics that should never be shared because every replica must observe them.

Acceptance criteria:
- automation workflow execution, command-result handling, and WebSocket run-event delivery remain correct with multiple replicas.
- device-hub remains exclusive until adapter, pairing, and command transition state are externally owned.
- no topic is moved to shared mode without an idempotency decision recorded in code or docs.
- each changed subscription has an owner and explicit telemetry coverage.

### Phase 6: adapter topology decision

Services:
- `mock-adapter`
- `zigbee-adapter`

Tasks:
- decide whether each adapter is an HA consumer, a singleton bridge, or a hybrid
- only adopt shared subscriptions where the adapter role is safe for distributed handling

Acceptance criteria:
- adapter deployment model is explicit
- broker-facing bridge responsibilities are not accidentally distributed without ownership rules

Current decision:

| Service | Profile | Subscription mode | Reason |
|---|---|---|---|
| `zigbee-adapter` | Exclusive singleton bridge | Exclusive | It owns the Zigbee coordinator connection, bridge event stream, and local pairing permit lifecycle. |
| `mock-adapter` | Exclusive singleton test adapter | Exclusive | Its synthetic pairing flow is process-local and exists for deterministic development/testing behavior. |

Both wrappers now explicitly request `SubscriptionModeExclusive`; no shared-group configuration is exposed for either adapter. Converting either adapter requires a separate distributed bridge design, not merely a broker subscription change.

### Phase 7: optional cross-repo convergence

Only do this after the core repo is stable.

Possible targets:
- `homenavi-connector`
- local Homenavi-broker paths in `homenavi-lg-thinq`
- `homenavi-emodul` if it benefits from standardization even without shared subscriptions

Tasks:
- adopt the broker-neutral helper only where it reduces maintenance cost without constraining specialized behavior
- preserve repo-local escape hatches for vendor-specific websocket, TLS, and protocol settings

Acceptance criteria:
- shared abstractions are reused where they genuinely simplify code
- specialized external MQTT clients still have a supported implementation path

## Kubernetes rollout plan

### Step 1: refactor `shared/mqttx` into a broker-neutral port with a Paho adapter
- Add neutral message/client/subscription abstractions.
- Keep Paho as the first concrete transport implementation.
- Add tests for reconnect replay using subscription intent rather than raw rewritten topics.
- Confirm the new client abstraction still supports exclusive singleton adapters and advanced transport options.

### Step 2: add broker strategy support
- Add EMQX strategy for shared subscriptions.
- Add generic strategy that explicitly rejects unsupported shared mode.
- Confirm config-driven selection works without service code changes.

### Step 3: convert one low-risk consumer service at a time
- Start with `history-service` or `entity-registry-service`.
- Confirm duplicate processing does not occur when replicas > 1.
- Keep the other services on exclusive mode during this stage.

### Step 4: scale up validated stateless consumers
- Increase replica counts only for already-converted services.
- Add HPA only after idempotency and reconnect behavior are verified.

### Step 5: broaden to the remaining consumers
- Convert `automation-service` and `device-hub` after the first service proves stable.
- Keep adapter services conservative until their state model is fully stateless.

### Step 6: decide cross-repo adoption separately
- Do not couple core rollout success to immediate integration-repo migration.
- Reuse the new client layer in adjacent repos only where their MQTT role matches the abstraction.

## Verification checklist
- Inventory tests: verify every known client setup is classified as shared-consumer, exclusive singleton, or specialized external transport.
- Unit tests for neutral topic strategy resolution.
- Unit tests for shared-subscription rejection on unsupported broker kinds.
- Unit tests for reconnect replay preserving subscription mode, group, and QoS.
- Unit tests for service-local wrappers using fake clients instead of concrete Paho-backed clients.
- Unit tests for advanced transport option mapping where supported by the common client layer.
- Unit tests that singleton adapter profiles preserve clean-session or resume-subscription behavior.
- Run the same message stream against 2+ replicas.
- Confirm each message is handled once per group, not once per pod.
- Confirm restarts do not cause duplicate side effects.
- Confirm backpressure and reconnect behavior are stable.
- Confirm the consumer code still works when a pod is evicted and replaced.

Broker-agnostic verification:
- Confirm service code does not construct broker-specific topic syntax.
- Confirm service code can be tested against an in-memory fake client that implements the neutral interface.
- Confirm switching from `emqx` strategy to `generic` strategy fails only for shared-mode consumers and not for standard publish/subscribe behavior.
- Confirm specialized external clients are either intentionally excluded from the common shared-subscription path or supported through explicit advanced transport features.

Enterprise-operability verification:
- Confirm metrics and logs distinguish transport failure from handler failure.

## Detailed test plan

### Shared package tests

Files:
- `shared/mqttx/mqtt_test.go`
- new tests such as `shared/mqttx/topic_strategy_test.go`
- new tests such as `shared/mqttx/client_replay_test.go`
- new tests such as `shared/mqttx/paho_transport_test.go`

Test cases:
- normalize broker URLs for tcp, tls, ws, and wss
- reject unsupported broker URL schemes
- resolve EMQX shared topics correctly
- reject shared mode on generic strategy
- preserve exclusive mode topic unchanged
- reject shared mode when group is empty
- resubscribe after reconnect using original `SubscriptionOptions`
- preserve QoS across replay
- publish with qos/retain options mapped correctly to the transport adapter
- propagate `OnConnect` and `OnConnectionLost` hooks correctly
- preserve clean-session and resume-subscription behavior for singleton profiles
- support advanced transport option translation for websocket headers, TLS config, and protocol version when that path is enabled
- emit the expected telemetry labels and events for connect, reconnect, subscribe, replay, and failure paths

### Core service wrapper tests

Files to add or update:
- `automation-service/internal/infra/mqtt/client_test.go`
- `device-hub/internal/infra/mqtt/client_test.go`
- `entity-registry-service/internal/infra/mqtt/client_test.go`
- `history-service/internal/infra/mqtt/client_test.go`
- `zigbee-adapter/internal/infra/mqtt/client_test.go`
- `mock-adapter/internal/adapter/service_test.go`

Test cases:
- wrappers subscribe using the expected `SubscriptionOptions`
- wrappers publish with the expected qos/retain flags
- wrappers do not expose or require Paho concrete types
- fake clients can drive the wrapper behavior in unit tests
- singleton adapters preserve their current session semantics after migration
- wrapper errors preserve enough context for operator debugging

### Consumer behavior tests

History-service:
- duplicate message replay does not create incorrect duplicate side effects beyond the allowed append/idempotent model
- retained-message handling stays consistent with current `AllowRetains` behavior
- shared-group subscription uses the expected group name

Entity-registry-service:
- repeated metadata/state/event deliveries do not break autoimport correctness
- retained bootstrap still works under shared mode if intended
- reconnect replay re-subscribes all required retained streams
- shared-mode decision records cover retained bootstrap semantics explicitly

Automation-service:
- shared subscriptions do not cause duplicate workflow execution for the same correlation or entity state transition
- reconnect triggers any required resync handlers once
- non-shareable subscriptions remain exclusive
- observability distinguishes engine-side handler failures from transport failures

Device-hub:
- event subscriptions selected for shared mode still reconcile command results and pairing progress correctly
- topics that must remain exclusive are not accidentally converted
- retained metadata/state publication behavior is unchanged
- subscription classification exists for every MQTT topic family used by device-hub

Zigbee-adapter and mock-adapter:
- neutral-client migration preserves existing command and pairing handling
- no shared-subscription rollout occurs unless explicitly enabled by topology decision

### Integration repo tests and validation

Connector:
- subscribe-on-connect behavior remains intact if it adopts the shared helper pattern
- hello/status publish on reconnect still happens exactly once per reconnect event

Emodul:
- singleton bridge reconnect and sync loop remain unchanged if only the neutral client pattern is adopted
- no shared-subscription behavior is introduced unless intentionally designed later

LG ThinQ local broker client:
- local Homenavi broker connection preserves TLS and retry behavior if migrated to a shared helper

LG ThinQ external realtime bridge:
- explicit test coverage should prove either that it remains outside the common client path or that advanced transport options fully preserve websocket headers, TLS, default publish handler, and manual reconnect behavior

### Integration and system tests

Environment:
- EMQX with 2+ replicas of the target consumer service where applicable
- representative retained messages and live event streams

Scenarios:
- two replicas of `history-service` share one ingest group and each message is handled once per group
- two replicas of `entity-registry-service` share one autoimport group and do not create inconsistent duplicate entities
- rolling restart during active traffic preserves subscription replay and does not multiply side effects
- broker disconnect/reconnect preserves subscription mode and handler behavior
- scaling a service back down does not strand required consumer responsibility

### Regression gate before enabling shared mode per service

For each service:
- unit tests for wrapper and handler semantics are green
- service integration tests are green
- an explicit decision exists for each subscription: shared, exclusive, or deferred
- observability confirms connect, disconnect, resubscribe, and handler error metrics are visible

## Acceptance criteria
- Core MQTT consumers can run with more than one replica.
- Shared subscriptions are centralized in a broker strategy inside the shared MQTT helper.
- Message handling is idempotent enough for pod churn and retries.
- The platform can scale event consumers independently from producers.
- The public `shared/mqttx` API is broker-neutral and does not expose Paho-specific public types.
- Service code expresses subscription intent such as shared vs exclusive without embedding EMQX syntax.
- Operators can observe transport health, replay health, and handler health separately.
- Every migrated subscription has a documented semantic decision, test coverage, and an owning service decision.
