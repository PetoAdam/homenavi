# Homenavi Helm Chart (Scaffold)

This chart provides an initial Kubernetes deployment path for Homenavi core services and infrastructure dependencies.

## Scope

The scaffold currently includes deployable defaults for:

- frontend
- api-gateway
- auth-service
- user-service
- dashboard-service
- integration-proxy
- email-service
- profile-picture-service
- postgres
- redis

Each component is controlled through `values.yaml` under `services.<name>.enabled`.

## Install

```bash
helm upgrade --install homenavi ./helm/homenavi -n homenavi --create-namespace
```

## Validate

```bash
helm lint ./helm/homenavi
helm template homenavi ./helm/homenavi > /tmp/homenavi-rendered.yaml
```

## Notes

- Default values are intentionally conservative and intended as a baseline scaffold.
- Sensitive values should be provided via `services.<name>.envFromSecrets`.
- Persistent storage can be enabled through `persistentVolumeClaims` and referenced by service volumes.
- `integration-proxy` defaults to `INTEGRATIONS_RUNTIME_MODE=helm` in this chart.

## Runtime secrets from env file (Kubernetes-native)

Kubernetes/Helm does not auto-load `.env` files like Docker Compose.

Use a Kubernetes secret created from an env file and inject it globally:

```bash
kubectl -n homenavi create secret generic homenavi-runtime-env \
	--from-env-file=/home/adam/Projects/homenavi/.env \
	--dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install homenavi ./helm/homenavi -n homenavi \
	--set global.envFromSecrets[0]=homenavi-runtime-env
```

The chart applies `global.envFromSecrets` to all enabled services via `envFrom`.

## Optional: bridge to external Zigbee2MQTT broker

If you already run Zigbee2MQTT elsewhere, you can bridge this chart's Mosquitto to that broker.

1. Create a local bridge config file (gitignored in this repo):

```bash
mkdir -p mosquitto/config/conf.d
cat > mosquitto/config/conf.d/bridge.conf <<'EOF'
connection zigbee-prod
address 192.168.64.141:1883
bridge_protocol_version mqttv311
try_private false
cleansession false
remote_clientid homenavi-zigbee-bridge-local
keepalive_interval 60
restart_timeout 5 30
topic zigbee2mqtt/+ in 1
topic zigbee2mqtt/+/availability in 1
topic zigbee2mqtt/bridge/# in 1
topic zigbee2mqtt/+/set out 1
topic zigbee2mqtt/+/set/# out 1
topic zigbee2mqtt/bridge/request/# out 1
EOF
```

2. Enable bridge config during install/upgrade:

```bash
helm upgrade --install homenavi ./helm/homenavi -n homenavi \
	--set services.mosquitto.bridge.enabled=true \
	--set-file services.mosquitto.bridge.config=./mosquitto/config/conf.d/bridge.conf
```

By default, bridge mode is disabled and no external broker link is created.

Important: avoid `topic zigbee2mqtt/# both 1` unless you really need full mirroring. It can create command/state echo loops and high-rate Zigbee2MQTT converter errors on constrained devices (e.g. Raspberry Pi).

## Deployment mode

This chart is maintained for the single-namespace local model (`homenavi`, plus `homenavi-marketplace` for marketplace).

Legacy multi-plane values profiles were removed.
