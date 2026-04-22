# Homenavi Helm Chart

This chart provides an initial Kubernetes deployment path for Homenavi core services and infrastructure dependencies.

Released versions are published to GHCR as an OCI Helm chart:

- `oci://ghcr.io/petoadam/charts/homenavi`

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
- emqx
- minio
- postgres
- redis

Each component is controlled through `values.yaml` under `services.<name>.enabled`.

## Install

Released chart:

```bash
helm install homenavi oci://ghcr.io/petoadam/charts/homenavi \
	--version X.Y.Z \
	-n homenavi --create-namespace
```

Local chart checkout:

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
- The default bundled MQTT provider is EMQX. Keep EMQX as the primary broker and prefer direct bridges into it when integrating external MQTT deployments.
- The default bundled PostgreSQL provider is CloudNativePG.
- The default bundled Redis mode is Sentinel.
- The default profile-picture backend is MinIO-backed S3 storage.
- Bundled dependency startup in Kubernetes is handled with readiness/startup probes plus narrow init-container dependency waits, rather than strict global startup ordering.
- Sensitive values should be provided via `services.<name>.envFromSecrets`.
- Per-service secret or configMap-backed env vars can also be injected with `services.<name>.envValueFrom`.
- Persistent storage can be enabled through `persistentVolumeClaims` and referenced by service volumes.
- `integration-proxy` defaults to `INTEGRATIONS_RUNTIME_MODE=helm` in this chart.
- Released charts default service image tags to the chart `appVersion`, so tag-based releases stay aligned with GHCR images by default.

## Runtime secrets from env file (Kubernetes-native)

Kubernetes/Helm does not auto-load `.env` files like Docker Compose.

Use a Kubernetes secret created from an env file and inject it globally:

```bash
kubectl -n homenavi create secret generic homenavi-runtime-env \
	--from-env-file=./.env \
	--dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install homenavi ./helm/homenavi -n homenavi \
	--set global.envFromSecrets[0]=homenavi-runtime-env
```

The chart applies `global.envFromSecrets` to all enabled services via `envFrom`.

## Optional: load EMQX bridge snippets

The chart defaults to EMQX for Homenavi services. If you need broker bridging, add bridge snippets under `services.emqx.bridgeConfigFiles`.

See [doc/mqtt_broker_topologies.md](../../doc/mqtt_broker_topologies.md) for the preferred topology and bridge-direction guidance.

Use [emqx/bridge.d/homenavi-bridge.example.hocon](../../emqx/bridge.d/homenavi-bridge.example.hocon) as the starting point for custom snippets.

Example values override:

```yaml
services:
	emqx:
		bridgeConfigFiles:
			20-external-zigbee.hocon: |
				## Example only.
				## Copy the commented starter from emqx/bridge.d/homenavi-bridge.example.hocon
				## and then enable just the connector/action/source/rule blocks you need.
```

Apply it with:

```bash
helm upgrade --install homenavi ./helm/homenavi -n homenavi -f custom-values.yaml
```

By default, no external bridge is created.

## External dependency secrets and HA policy primitives

The chart supports existing secret references for PostgreSQL, Redis, and S3/MinIO credentials, plus raw Kubernetes scheduling/policy primitives for later scale-out.

See [doc/helm_ha_operations.md](../../doc/helm_ha_operations.md) for:

- external PostgreSQL / Redis / storage secret reference patterns
- CNPG backup and recovery checks
- Redis Sentinel failover checks
- optional PodDisruptionBudget, anti-affinity, topology spread, and NetworkPolicy examples

## Deployment mode

This chart is maintained for the single-namespace local model. End users install only Homenavi itself; the marketplace is a separate central service used by all Homenavi installations.

Legacy multi-plane values profiles were removed.
