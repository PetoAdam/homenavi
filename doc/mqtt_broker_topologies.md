# MQTT broker defaults and bridge topologies

Homenavi now treats **EMQX** as the primary and recommended MQTT broker in both Docker Compose and Helm.

## 1. Default stance

Use this mental model:

- **EMQX** = main Homenavi broker
- **direct broker-to-EMQX bridge** = preferred interop path

That keeps the Homenavi runtime simple while still allowing bridge-based migrations.

## 2. Preferred topology: direct bridge into EMQX

When the external broker can be configured, prefer this:

```text
external broker  <---- MQTT bridge ---->  Homenavi EMQX
```

Why this is preferred:

- fewer moving parts
- fewer containers to run and debug
- fewer places for loops or duplicate subscriptions
- clearer ownership of the primary Homenavi broker

This usually means **configuring the external broker**, not Homenavi itself.

Examples:

- a remote broker bridges directly to Homenavi EMQX
- another EMQX instance bridges directly to Homenavi EMQX
- a vendor-specific broker forwards selected topics directly into Homenavi EMQX

For Homenavi, the stable listener to target is plain MQTT/TCP on port `1883`.

Important: broker bridging uses normal MQTT/TCP. It does **not** use the frontend WebSocket endpoint.

## 3. Bridge snippets managed by Homenavi

When Homenavi itself should own the bridge configuration, store bridge snippets as `.hocon` files.

### Docker Compose

- Local snippets live in `emqx/bridge.d/*.hocon`
- That directory is gitignored except for `.gitkeep` and `*.example.hocon`
- The startup wrapper concatenates every snippet into `/opt/emqx/etc/homenavi-bridge.hocon`

Starter file:

- `emqx/bridge.d/homenavi-bridge.example.hocon`

### Helm

Provide snippets through `services.emqx.bridgeConfigFiles`.

Example:

```yaml
services:
  emqx:
    bridgeConfigFiles:
      20-external-bridge.hocon: |
        ## enabled bridge config here
```

## 4. Topic ownership rules

Bridge loops and duplicate actuation are the real danger, not WebSockets.

Use these rules:

1. **State / metadata / events** should normally flow **out** from edge brokers into Homenavi.
2. **Commands** should normally flow **in** from exactly **one active control plane**.
3. If two Homenavi deployments are bridged to the same devices, let only one of them own command topics at a time.

For HDP and Zigbee2MQTT, that usually means:

- outbound on:
  - `homenavi/hdp/adapter/hello`
  - `homenavi/hdp/adapter/status/#`
  - `homenavi/hdp/device/metadata/#`
  - `homenavi/hdp/device/state/#`
  - `homenavi/hdp/device/event/#`
  - `homenavi/hdp/device/command_result/#`
  - `homenavi/hdp/pairing/progress/#`
  - `zigbee2mqtt/+`
  - `zigbee2mqtt/+/availability`
  - `zigbee2mqtt/bridge/#`
- inbound only for the active controller on:
  - `homenavi/hdp/device/command/#`
  - `homenavi/hdp/pairing/command/#`
  - `zigbee2mqtt/+/set`
  - `zigbee2mqtt/+/set/#`
  - `zigbee2mqtt/bridge/request/#`

Avoid broad bidirectional rules like `topic zigbee2mqtt/# both 1` unless you are intentionally mirroring everything and have fully reasoned through loops.

## 5. Practical migration notes

If an external Raspberry Pi or another host already runs its own broker, configure the bridge on that external broker directly and target the Homenavi EMQX listener on port `1883`.

For migration safety:

1. forward telemetry topics first
2. keep command topics disabled until the new stack is ready to own control
3. enable exactly one active command publisher during cutover

## 6. Practical notes

- If a target deployment is not directly reachable on port `1883`, use the real reachable broker endpoint instead of the public web URL.
- Do not point broker bridges at `/ws/hdp`; that endpoint is only for frontend/browser clients.
- If authentication is enabled on the target broker, add `remote_username` and `remote_password` to the bridge file.
- If you need the old and new Homenavi stacks to coexist for a while, keep one passive for commands and active for telemetry.

## 7. Summary

Recommended order:

1. keep EMQX as the default Homenavi broker
2. prefer a direct bridge from the external broker into EMQX
3. keep bridge snippets in `emqx/bridge.d/` for Compose or `services.emqx.bridgeConfigFiles` for Helm
4. during migration, fan out telemetry to both stacks but let only one stack own commands
