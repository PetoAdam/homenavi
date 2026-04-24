# Helm HA operations and secret reference patterns

This document covers the HA-oriented operational pieces for the Homenavi Helm chart.

## 1. Existing secret references for external or bundled dependencies

The chart supports Kubernetes `Secret` references for the most common dependency credentials instead of forcing them into plain Helm values.

### PostgreSQL

Use `postgres.auth.existingSecretName` when the database credentials already live in a Kubernetes secret.

Expected keys by default:

- `username`
- `password`

Example:

```yaml
dependencies:
  postgres:
    provider: external

postgres:
  mode: external
  host: app-postgres-rw.database.svc
  port: "5432"
  database: homenavi
  sslMode: disable
  auth:
    existingSecretName: external-postgres
    usernameKey: username
    passwordKey: password
```

Application pods and the schema bootstrap job will read `POSTGRES_USER` and `POSTGRES_PASSWORD` from that secret.

### Redis

Use `redis.auth.existingSecretName` for standalone or Sentinel deployments when Redis requires a password.

Example:

```yaml
dependencies:
  redis:
    provider: external

redis:
  mode: sentinel
  sentinelAddrs:
    - redis-sentinel-0.redis.svc:26379
    - redis-sentinel-1.redis.svc:26379
    - redis-sentinel-2.redis.svc:26379
  masterName: redis-prod
  auth:
    existingSecretName: external-redis
    passwordKey: password
```

Services that use Redis will receive `REDIS_PASSWORD` from the referenced secret automatically.

### S3 / MinIO storage

Use `storage.s3.existingSecretName` to source the object storage access and secret keys from a Kubernetes secret.

Expected keys by default:

- `accessKey`
- `secretKey`

Example with external S3-compatible storage:

```yaml
dependencies:
  objectStorage:
    provider: external

storage:
  type: s3
  s3:
    endpoint: https://minio.example.internal
    region: us-east-1
    bucket: profile-pictures
    forcePathStyle: true
    existingSecretName: external-storage
    accessKeyKey: accessKey
    secretKeyKey: secretKey
```

The chart uses those secret refs for:

- `profile-picture-service`
- the profile picture bucket bootstrap init container
- bundled MinIO when `dependencies.objectStorage.provider=minio`

When `storage.s3.existingSecretName` is set, the chart treats that secret as the only source of truth for storage credentials. It does not render a managed `*-storage-auth` secret in that mode.

### Generic per-service `envValueFrom`

Each service also supports explicit `envValueFrom` entries for one-off secret or config map references.

Example:

```yaml
services:
  weather-service:
    envValueFrom:
      OPENWEATHER_API_KEY:
        secretKeyRef:
          name: weather-api
          key: apiKey
```

## 2. CloudNativePG backup / recovery guidance

The chart installs a `Cluster` resource when PostgreSQL is bundled in CNPG mode. Basic operational checks:

```bash
kubectl -n homenavi get clusters.postgresql.cnpg.io
kubectl -n homenavi get pods -l app.kubernetes.io/component=postgres
kubectl -n homenavi get secret | grep postgres
```

Primary service endpoints created by CNPG:

- `<cluster-name>-rw` for reads/writes
- `<cluster-name>-ro` for read-only traffic when replicas exist

Recommended next production step:

- add CNPG `backup` / `scheduledBackup` manifests that target either an object store or volume snapshots

For now, recovery validation should at least include:

```bash
kubectl -n homenavi describe cluster <cluster-name>
kubectl -n homenavi get pods
kubectl -n homenavi logs job/<release>-schema-bootstrap
```

## 3. Redis Sentinel failover checks

Basic checks:

```bash
kubectl -n homenavi get pods -l app.kubernetes.io/component=redis
kubectl -n homenavi get svc | grep redis
```

Inspect the active master from a Sentinel pod:

```bash
kubectl -n homenavi exec -it <redis-pod> -c sentinel -- redis-cli -p 26379 sentinel master homenavi-redis
```

Inspect replica state from a Redis pod:

```bash
kubectl -n homenavi exec -it <redis-pod> -c redis -- redis-cli info replication
```

For non-production testing, a simple failover exercise is:

1. identify the current master pod
2. delete that pod
3. confirm Sentinel elects a new master
4. confirm Homenavi services reconnect without manual reconfiguration

## 4. Optional HA scheduling and policy primitives

These are available now, but intentionally left opt-in while services stay on a single replica.

### PodDisruptionBudget

Example:

```yaml
services:
  api-gateway:
    podDisruptionBudget:
      enabled: true
      maxUnavailable: 1
```

Use this after moving a service beyond one replica. With one replica, a strict PDB can block voluntary evictions.

### Anti-affinity

Example:

```yaml
services:
  api-gateway:
    affinity:
      podAntiAffinity:
        preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              topologyKey: kubernetes.io/hostname
              labelSelector:
                matchLabels:
                  app.kubernetes.io/name: homenavi
                  app.kubernetes.io/component: api-gateway
```

### Topology spread constraints

Example:

```yaml
services:
  frontend:
    topologySpreadConstraints:
      - maxSkew: 1
        topologyKey: kubernetes.io/hostname
        whenUnsatisfiable: ScheduleAnyway
        labelSelector:
          matchLabels:
            app.kubernetes.io/name: homenavi
            app.kubernetes.io/component: frontend
```

### NetworkPolicy

Example:

```yaml
services:
  api-gateway:
    networkPolicy:
      enabled: true
      policyTypes:
        - Ingress
        - Egress
      ingress:
        - from:
            - podSelector:
                matchLabels:
                  app.kubernetes.io/name: homenavi
      egress:
        - to:
            - namespaceSelector: {}
```

These settings are deliberately raw Kubernetes primitives so the chart does not over-assume cluster topology.
