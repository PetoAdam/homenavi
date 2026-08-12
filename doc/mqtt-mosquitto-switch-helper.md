# Homenavi Mosquitto Switch Helper

## Purpose
Provide a short compatibility guide for people who want to run Homenavi with Mosquitto instead of EMQX.

## Key point
The HA design should rely on standard MQTT shared-subscription syntax and stateless consumers, not EMQX-specific features.

That means a Mosquitto deployment can work if the broker version and deployment topology support the topics and session behavior you need.

## What should stay the same
- MQTT topic layout.
- Shared subscription syntax.
- Consumer idempotency.
- Stateless service behavior.
- Kubernetes replica scaling for the consumers.

## What changes when swapping brokers
- Broker service name and URL.
- Broker config and credentials.
- Any EMQX-specific admin or clustering assumptions.
- Any broker-specific dashboards or management URLs.

## Suggested migration steps
1. Replace the broker service in the deployment manifests.
2. Point `MQTT_BROKER_URL` at the Mosquitto service.
3. Keep the same topic layout and shared-subscription groups.
4. Re-test consumer scaling with 2+ replicas.
5. Verify retained messages, reconnect behavior, and clean-session settings.

## Caveats
- Mosquitto is usually a simpler broker story than EMQX, but it gives you fewer built-in management and clustering conveniences.
- If your deployment depends heavily on broker-level multi-node behavior, EMQX will likely remain the easier fit.
- If you only need standard MQTT plus shared subscriptions, Mosquitto can be a workable option.

## Practical recommendation
Keep the platform broker-agnostic in application code, but allow EMQX-specific deployment defaults in the main Kubernetes manifests.
