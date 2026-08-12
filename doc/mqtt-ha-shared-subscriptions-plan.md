# Homenavi MQTT Shared Subscriptions and HA Plan

## Goal
Make the event-driven parts of Homenavi horizontally scalable by using EMQX shared subscriptions and stateless consumers, so services can run with multiple replicas without duplicate message processing.

## Current state
The repo currently uses direct MQTT subscriptions in consumers such as:
- `device-hub`
- `automation-service`
- `entity-registry-service`
- `history-service`
- `mock-adapter`
- `zigbee-adapter`

There is no shared-subscription abstraction in the current shared MQTT wrapper yet.

## What shared subscriptions solve
With shared subscriptions, multiple pods can subscribe to the same logical stream and EMQX will deliver each message to only one member of the group.

That gives you:
- Horizontal scaling for consumers.
- Less duplication when scaling replicas.
- A clean path to stateless services.

## Important design rule
Shared subscriptions are not a blanket replacement for every topic.

Use them for work queues and fan-out processing where one consumer should handle each message once.
Do not use them for topics where every replica must see the same message.

## Recommended topic strategy

### Shared consumer groups
Use a group name per semantic workload, not one global group for everything.

Examples:
- `device-hub-hdp-events`
- `automation-engine-events`
- `history-ingest`
- `entity-registry-autoimport`
- `mock-adapter-command-handlers`
- `zigbee-adapter-command-handlers`

### Topic categories
- Device state / metadata ingestion can be shared when the handler is idempotent.
- Command handling can be shared if commands are already correlated to a single logical target.
- Integration import or refresh jobs can be shared as long as the job is replay-safe.

### Topics that may remain exclusive
- Broadcast-style notifications that must reach every replica.
- Any topic that drives in-memory caches that are not yet externalized.

## Services that should be converted first

### device-hub
- Convert event processing subscriptions to shared subscriptions.
- Keep adapter hello/status handling stateless.
- Make command-result and pairing-progress ingestion replay-safe.

### automation-service
- Convert MQTT state and command-result consumers to shared subscriptions.
- Ensure workflow execution is idempotent or deduplicated by correlation ID.

### history-service
- Convert MQTT ingest subscriptions to shared subscriptions.
- Keep writes append-only and idempotent.

### entity-registry-service
- Convert autoimport subscriptions to shared subscriptions.
- Ensure registry refresh is safe to re-run.

### mock-adapter and zigbee-adapter
- If these act as consumers in the final HA topology, use shared subscriptions for their handler groups as well.
- If they remain broker-facing adapters with a single logical role, they may stay single-replica initially.

## Shared MQTT wrapper change
Add helper support in `shared/mqttx` so services do not hand-build shared-subscription topic strings everywhere.

Recommended helper shape:
- `SharedTopic(group, topic string) string`
- `SubscribeShared(group, topic string, qos byte, cb Handler) error`
- `SubscribeSharedFunc(group, topic string, qos byte, cb func(Message)) error`

This keeps the `$share/<group>/<topic>` syntax centralized and testable.

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

## Kubernetes rollout plan

### Step 1: add shared-subscription helpers
- Update the shared MQTT wrapper first.
- Add unit tests for shared-topic formatting and subscription registration.

### Step 2: convert one consumer service at a time
- Start with history-service or entity-registry-service.
- Confirm duplicate processing does not occur when replicas > 1.

### Step 3: scale up stateless services
- Increase replica counts in Kubernetes for the converted services.
- Add HPA only after idempotency is verified.

### Step 4: broaden to the remaining consumers
- Convert automation-service and device-hub after the first service proves stable.
- Keep adapter services conservative until their state model is fully stateless.

## Verification checklist
- Run the same message stream against 2+ replicas.
- Confirm each message is handled once per group, not once per pod.
- Confirm restarts do not cause duplicate side effects.
- Confirm backpressure and reconnect behavior are stable.
- Confirm the consumer code still works when a pod is evicted and replaced.

## Acceptance criteria
- Core MQTT consumers can run with more than one replica.
- Shared subscriptions are centralized in the shared MQTT helper.
- Message handling is idempotent enough for pod churn and retries.
- The platform can scale event consumers independently from producers.
