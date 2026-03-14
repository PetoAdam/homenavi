# Minikube Helm MVP Runbook (Core + Marketplace)

Status: MVP-1 target  
Date: 2026-03-01

## Goal

First real MVP target:

1. Run the full Homenavi core stack on local Minikube via Helm.
2. Run a local Homenavi Marketplace instance on the same Minikube cluster via Helm.
3. Defer CI/verification hardening to post-MVP.

## Scope

In scope now:

- local cluster operability
- Helm-based install/upgrade flows
- smoke checks for core and marketplace

Out of scope for MVP-1:

- full CI quality gates
- advanced verification/provenance workflows
- GitOps automation

## Prerequisites

- `minikube`
- `kubectl`
- `helm`
- local Docker daemon available for Minikube image pulls/builds

## 1) Start local cluster

```bash
minikube start
kubectl config use-context minikube
kubectl get nodes
```

## 1.1 One-command deploy (recommended)

Use the repo script to deploy MVP in a single namespace (`homenavi`) plus marketplace:

```bash
cd /home/adam/Projects/homenavi
./scripts/deploy-minikube.sh
```

Optional flags:

```bash
# if marketplace local images are already built
./scripts/deploy-minikube.sh --skip-marketplace-build

# custom minikube profile
./scripts/deploy-minikube.sh --profile minikube

# keep previously deployed plane releases (default behavior is cleanup)
./scripts/deploy-minikube.sh --no-cleanup-planes

# enable mosquitto bridge config from local file
./scripts/deploy-minikube.sh --with-bridge

# use explicit env file for Kubernetes runtime secret ingestion
./scripts/deploy-minikube.sh --env-file /home/adam/Projects/homenavi/.env

# auto-start frontend + marketplace port-forwards on first free ports
./scripts/deploy-minikube.sh --start-port-forwards
```

The script now suggests (and optionally starts) consistent preferred host ports:

- frontend: `50001`
- marketplace: `50010`

If either preferred port is busy, it automatically picks the next free port.

The script creates/updates a Kubernetes secret (`homenavi-runtime-env` by default) from the env file and injects it into all chart services via `envFrom`.

The rest of this runbook documents the equivalent manual commands.

Create namespaces (recommended even when using `--create-namespace`):

```bash
kubectl create namespace homenavi --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace homenavi-marketplace --dry-run=client -o yaml | kubectl apply -f -
```

## 2) Deploy Homenavi core via Helm

Current chart path:

- `helm/homenavi`

Create local JWT keys (required by auth-service, api-gateway, and integration-proxy):

```bash
mkdir -p /tmp/homenavi-keys
openssl genrsa -out /tmp/homenavi-keys/jwt_private.pem 2048
openssl rsa -in /tmp/homenavi-keys/jwt_private.pem -pubout -out /tmp/homenavi-keys/jwt_public.pem
```

Install/upgrade core chart:

```bash
cd /home/adam/Projects/homenavi
helm upgrade --install homenavi ./helm/homenavi \
	-n homenavi --create-namespace \
	--set-file jwt.privateKey=/tmp/homenavi-keys/jwt_private.pem \
	--set-file jwt.publicKey=/tmp/homenavi-keys/jwt_public.pem
```

Quick checks:

```bash
kubectl -n homenavi get pods
kubectl -n homenavi get svc
kubectl -n homenavi get deploy
```

Optional local access checks:

```bash
kubectl -n homenavi port-forward svc/frontend 3000:80
kubectl -n homenavi port-forward svc/api-gateway 8080:8080
```

## 3) Deploy Homenavi Marketplace via Helm

MVP requirement: marketplace must be deployed by Helm in-cluster (same Minikube).

Expected chart target:

- repository: `homenavi-marketplace`
- chart path: `helm/homenavi-marketplace`
- namespace: `homenavi-marketplace`

Install/upgrade target command:

```bash
cd /home/adam/Projects/homenavi-marketplace
helm upgrade --install homenavi-marketplace ./helm/homenavi-marketplace -n homenavi-marketplace --create-namespace
```

Quick checks:

```bash
kubectl -n homenavi-marketplace get pods
kubectl -n homenavi-marketplace get svc
kubectl -n homenavi-marketplace get deploy
```

Optional API/UI reachability checks:

```bash
kubectl -n homenavi-marketplace port-forward svc/homenavi-marketplace-api 8098:8098
curl -fsS http://127.0.0.1:8098/api/health

kubectl -n homenavi-marketplace port-forward svc/homenavi-marketplace 3010:80
# open http://127.0.0.1:3010
```

## 4) Wire core to local marketplace

Set Homenavi integration-proxy marketplace API base to the in-cluster marketplace service URL:

- `INTEGRATIONS_MARKETPLACE_API_BASE=http://homenavi-marketplace.homenavi-marketplace.svc.cluster.local/api`

Then run:

```bash
kubectl -n homenavi rollout restart deploy/homenavi-integration-proxy
kubectl -n homenavi rollout status deploy/homenavi-integration-proxy
```

Alternative (Helm values update + upgrade):

```bash
cd /home/adam/Projects/homenavi
helm upgrade --install homenavi ./helm/homenavi \
	-n homenavi \
	--set-file jwt.privateKey=/tmp/homenavi-keys/jwt_private.pem \
	--set-file jwt.publicKey=/tmp/homenavi-keys/jwt_public.pem \
	--set services.integration-proxy.env.INTEGRATIONS_MARKETPLACE_API_BASE=http://homenavi-marketplace.homenavi-marketplace.svc.cluster.local/api
```

## 5) MVP smoke checklist

- Core pods are `Running` in namespace `homenavi`
- Marketplace pods are `Running` in namespace `homenavi-marketplace`
- Core frontend/API reachable via port-forward
- Marketplace API health endpoint responds
- Marketplace UI reachable via service port-forward
- Integration proxy can query marketplace integrations endpoint

## 5.1 Optional: bridge to an existing Zigbee2MQTT broker

If you already run Zigbee2MQTT outside this cluster, you can bridge Homenavi's in-cluster Mosquitto to that external broker.

Create local bridge config (kept out of git by `.gitignore`):

```bash
mkdir -p /home/adam/Projects/homenavi/mosquitto/config/conf.d
cat > /home/adam/Projects/homenavi/mosquitto/config/conf.d/bridge.conf <<'EOF'
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

Apply with Helm:

```bash
cd /home/adam/Projects/homenavi
helm upgrade --install homenavi ./helm/homenavi \
	-n homenavi \
	--set-file jwt.privateKey=/tmp/homenavi-keys/jwt_private.pem \
	--set-file jwt.publicKey=/tmp/homenavi-keys/jwt_public.pem \
	--set services.mosquitto.bridge.enabled=true \
	--set-file services.mosquitto.bridge.config=./mosquitto/config/conf.d/bridge.conf
```

Disable later by setting `services.mosquitto.bridge.enabled=false` and upgrading again.

## 5.2 Secrets management for Helm (from local env file)

Helm/Kubernetes does not read `.env` automatically like Docker Compose.

Recommended local workflow:

1. Copy and fill your local env values:

```bash
cp /home/adam/Projects/homenavi/k8s/secrets/homenavi.env.example /home/adam/Projects/homenavi/.env
```

2. Deploy with script (creates/updates runtime secret automatically):

```bash
cd /home/adam/Projects/homenavi
./scripts/deploy-minikube.sh --env-file /home/adam/Projects/homenavi/.env
```

Manual equivalent:

```bash
kubectl -n homenavi create secret generic homenavi-runtime-env \
	--from-env-file=/home/adam/Projects/homenavi/.env \
	--dry-run=client -o yaml | kubectl apply -f -

helm upgrade --install homenavi ./helm/homenavi \
	-n homenavi \
	--set-file jwt.privateKey=/tmp/homenavi-keys/jwt_private.pem \
	--set-file jwt.publicKey=/tmp/homenavi-keys/jwt_public.pem \
	--set global.envFromSecrets[0]=homenavi-runtime-env
```

This is the path to make features like weather API and Google OAuth receive runtime secrets in Kubernetes.

## 6) Post-MVP hardening (next phase)

- Add/extend CI (`helm lint`, `helm template`, smoke deploy job)
- Add runtime behavior integration tests
- Add verification/validation hardening and release checks

## 7) Deployment model note

The maintained local path is a single `homenavi` namespace (plus `homenavi-marketplace`).

Legacy multi-plane values profiles were removed to reduce drift and restart/debug complexity.

Use:

```bash
./scripts/deploy-minikube.sh --env-file /home/adam/Projects/homenavi/.env --start-port-forwards
```
