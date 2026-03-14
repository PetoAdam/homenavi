#!/usr/bin/env bash
set -euo pipefail

SKIP_MARKETPLACE_BUILD="false"
MINIKUBE_PROFILE="minikube"
CLEANUP_PLANES="true"
WITH_BRIDGE="false"
BRIDGE_CONFIG_FILE=""
ENV_FILE=""
RUNTIME_SECRET_NAME="homenavi-runtime-env"
START_PORT_FORWARDS="false"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-marketplace-build)
      SKIP_MARKETPLACE_BUILD="true"
      shift
      ;;
    --profile)
      MINIKUBE_PROFILE="${2:-}"
      if [[ -z "$MINIKUBE_PROFILE" ]]; then
        echo "--profile requires a value"
        exit 1
      fi
      shift 2
      ;;
    --no-cleanup-planes)
      CLEANUP_PLANES="false"
      shift
      ;;
    --with-bridge)
      WITH_BRIDGE="true"
      shift
      ;;
    --bridge-config-file)
      BRIDGE_CONFIG_FILE="${2:-}"
      if [[ -z "$BRIDGE_CONFIG_FILE" ]]; then
        echo "--bridge-config-file requires a value"
        exit 1
      fi
      shift 2
      ;;
    --env-file)
      ENV_FILE="${2:-}"
      if [[ -z "$ENV_FILE" ]]; then
        echo "--env-file requires a value"
        exit 1
      fi
      shift 2
      ;;
    --runtime-secret-name)
      RUNTIME_SECRET_NAME="${2:-}"
      if [[ -z "$RUNTIME_SECRET_NAME" ]]; then
        echo "--runtime-secret-name requires a value"
        exit 1
      fi
      shift 2
      ;;
    --start-port-forwards)
      START_PORT_FORWARDS="true"
      shift
      ;;
    *)
      echo "Unknown argument: $1"
      echo "Usage: $0 [--skip-marketplace-build] [--profile <minikube-profile>] [--no-cleanup-planes] [--with-bridge] [--bridge-config-file <path>] [--env-file <path>] [--runtime-secret-name <name>] [--start-port-forwards]"
      exit 1
      ;;
  esac
done

need_cmd() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "Missing required command: $1"
    exit 1
  fi
}

need_cmd minikube
need_cmd kubectl
need_cmd helm
need_cmd openssl
need_cmd docker
need_cmd ss

is_port_in_use() {
  local port="$1"
  ss -ltn "( sport = :$port )" | tail -n +2 | grep -q .
}

pick_first_free_port() {
  local start_port="$1"
  local port="$start_port"
  while is_port_in_use "$port"; do
    port=$((port + 1))
  done
  echo "$port"
}

to_abs_path() {
  local input_path="$1"
  if [[ -z "$input_path" ]]; then
    return 1
  fi
  if [[ "$input_path" = /* ]]; then
    echo "$input_path"
    return 0
  fi
  local dir_part
  dir_part="$(cd "$(dirname "$input_path")" && pwd)"
  echo "$dir_part/$(basename "$input_path")"
}

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
HOMENAVI_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MARKETPLACE_ROOT="$(cd "$HOMENAVI_ROOT/../homenavi-marketplace" && pwd)"
ENSURE_JWT_KEYS_SCRIPT="$SCRIPT_DIR/ensure-jwt-keys.sh"

if [[ -z "$ENV_FILE" ]]; then
  ENV_FILE="$HOMENAVI_ROOT/.env"
fi
if [[ -z "$BRIDGE_CONFIG_FILE" ]]; then
  BRIDGE_CONFIG_FILE="$HOMENAVI_ROOT/mosquitto/config/conf.d/bridge.conf"
fi

ENV_FILE="$(to_abs_path "$ENV_FILE")"
BRIDGE_CONFIG_FILE="$(to_abs_path "$BRIDGE_CONFIG_FILE")"

if ! minikube -p "$MINIKUBE_PROFILE" status >/dev/null 2>&1; then
  minikube -p "$MINIKUBE_PROFILE" start
fi

"$ENSURE_JWT_KEYS_SCRIPT" --dir /tmp/homenavi-keys

kubectl create namespace homenavi --dry-run=client -o yaml | kubectl apply -f -
kubectl create namespace homenavi-marketplace --dry-run=client -o yaml | kubectl apply -f -

if [[ -f "$ENV_FILE" ]]; then
  kubectl -n homenavi create secret generic "$RUNTIME_SECRET_NAME" \
    --from-env-file="$ENV_FILE" \
    --dry-run=client -o yaml | kubectl apply -f -
  echo "Applied runtime env secret: $RUNTIME_SECRET_NAME from $ENV_FILE"
else
  echo "Runtime env file not found at $ENV_FILE (skipping runtime env secret creation)."
fi

if [[ "$CLEANUP_PLANES" == "true" ]]; then
  helm -n homenavi-client uninstall homenavi-client >/dev/null 2>&1 || true
  helm -n homenavi-core uninstall homenavi-core >/dev/null 2>&1 || true
  helm -n homenavi-edge uninstall homenavi-edge >/dev/null 2>&1 || true
fi

eval "$(minikube -p "$MINIKUBE_PROFILE" docker-env)"

docker build -t ghcr.io/petoadam/homenavi-frontend:latest "$HOMENAVI_ROOT/frontend"

if [[ "$SKIP_MARKETPLACE_BUILD" != "true" ]]; then
  docker build -t homenavi-marketplace-api:local "$MARKETPLACE_ROOT/api"
  docker build -t homenavi-marketplace-web:local "$MARKETPLACE_ROOT/web"
fi

cd "$HOMENAVI_ROOT"
HELM_ARGS=(
  upgrade --install homenavi ./helm/homenavi
  -n homenavi
  --set-file jwt.privateKey=/tmp/homenavi-keys/jwt_private.pem
  --set-file jwt.publicKey=/tmp/homenavi-keys/jwt_public.pem
  --set global.imagePullPolicy=IfNotPresent
  --set persistentVolumeClaims.postgres-data.enabled=true
  --set persistentVolumeClaims.profile-pictures.enabled=true
  --set persistentVolumeClaims.mosquitto-data.enabled=true
  --set persistentVolumeClaims.mosquitto-logs.enabled=true
  --set persistentVolumeClaims.zigbee2mqtt-data.enabled=true
  --set services.frontend.image.repository=ghcr.io/petoadam/homenavi-frontend
  --set services.frontend.image.tag=latest
)

if [[ -f "$ENV_FILE" ]]; then
  HELM_ARGS+=(--set "global.envFromSecrets[0]=$RUNTIME_SECRET_NAME")
fi

if [[ "$WITH_BRIDGE" == "true" ]]; then
  if [[ ! -f "$BRIDGE_CONFIG_FILE" ]]; then
    echo "Bridge enabled but config file not found: $BRIDGE_CONFIG_FILE"
    exit 1
  fi
  HELM_ARGS+=(
    --set services.mosquitto.bridge.enabled=true
    --set-file "services.mosquitto.bridge.config=$BRIDGE_CONFIG_FILE"
  )
fi

helm "${HELM_ARGS[@]}"

cd "$MARKETPLACE_ROOT"
helm upgrade --install homenavi-marketplace ./helm/homenavi-marketplace \
  -n homenavi-marketplace \
  --set api.image.repository=homenavi-marketplace-api \
  --set api.image.tag=local \
  --set web.image.repository=homenavi-marketplace-web \
  --set web.image.tag=local \
  --set imagePullPolicy=IfNotPresent

echo ""
echo "Deployment summary:"
kubectl -n homenavi get pods
kubectl -n homenavi-marketplace get pods

echo ""
echo "Windows browser access:"
FRONTEND_PORT="$(pick_first_free_port 50001)"
MARKETPLACE_PORT="$(pick_first_free_port 50010)"
echo "  kubectl -n homenavi port-forward --address 0.0.0.0 svc/frontend ${FRONTEND_PORT}:80"
echo "  kubectl -n homenavi-marketplace port-forward --address 0.0.0.0 svc/homenavi-marketplace ${MARKETPLACE_PORT}:80"

if [[ "$START_PORT_FORWARDS" == "true" ]]; then
  FRONTEND_LOG="/tmp/homenavi-port-forward-frontend.log"
  MARKETPLACE_LOG="/tmp/homenavi-port-forward-marketplace.log"

  nohup kubectl -n homenavi port-forward --address 0.0.0.0 svc/frontend "${FRONTEND_PORT}:80" >"$FRONTEND_LOG" 2>&1 &
  FRONTEND_PF_PID=$!

  nohup kubectl -n homenavi-marketplace port-forward --address 0.0.0.0 svc/homenavi-marketplace "${MARKETPLACE_PORT}:80" >"$MARKETPLACE_LOG" 2>&1 &
  MARKETPLACE_PF_PID=$!

  sleep 1

  echo ""
  echo "Auto-started port-forwards:"

  if kill -0 "$FRONTEND_PF_PID" >/dev/null 2>&1; then
    echo "  Frontend:    http://127.0.0.1:${FRONTEND_PORT} (pid=${FRONTEND_PF_PID}, log=${FRONTEND_LOG})"
  else
    echo "  Frontend port-forward failed. See ${FRONTEND_LOG}"
  fi

  if kill -0 "$MARKETPLACE_PF_PID" >/dev/null 2>&1; then
    echo "  Marketplace: http://127.0.0.1:${MARKETPLACE_PORT} (pid=${MARKETPLACE_PF_PID}, log=${MARKETPLACE_LOG})"
  else
    echo "  Marketplace port-forward failed. See ${MARKETPLACE_LOG}"
  fi

  echo ""
  echo "To stop:"
  echo "  kill ${FRONTEND_PF_PID} ${MARKETPLACE_PF_PID}"
fi
