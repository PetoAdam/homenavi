#!/usr/bin/env bash
set -euo pipefail

SKIP_MARKETPLACE_BUILD="false"
DEPLOY_MARKETPLACE="false"
MINIKUBE_PROFILE="minikube"
CLEANUP_PLANES="true"
BRIDGE_CONFIG_FILE=""
ENV_FILE=""
RUNTIME_SECRET_NAME="homenavi-runtime-env"
START_PORT_FORWARDS="true"
LOCAL_REGISTRY_NAME="homenavi-local-registry"
LOCAL_REGISTRY_PUSH_HOST="localhost:5000"
LOCAL_REGISTRY_PULL_HOST="host.minikube.internal:5000"
LOCAL_IMAGE_TAG="${LOCAL_IMAGE_TAG:-local-$(date +%Y%m%d-%H%M%S)}"

while [[ $# -gt 0 ]]; do
  case "$1" in
    --skip-marketplace-build)
      SKIP_MARKETPLACE_BUILD="true"
      shift
      ;;
    --without-marketplace|--skip-marketplace)
      DEPLOY_MARKETPLACE="false"
      SKIP_MARKETPLACE_BUILD="true"
      shift
      ;;
    --with-marketplace)
      DEPLOY_MARKETPLACE="true"
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
    --no-start-port-forwards)
      START_PORT_FORWARDS="false"
      shift
      ;;
    *)
      echo "Unknown argument: $1"
      echo "Usage: $0 [--with-marketplace] [--skip-marketplace-build] [--profile <minikube-profile>] [--no-cleanup-planes] [--bridge-config-file <path>] [--env-file <path>] [--runtime-secret-name <name>] [--start-port-forwards] [--no-start-port-forwards]"
      echo "  Default behavior deploys only the core Homenavi stack."
      echo "  --with-marketplace also builds and deploys the marketplace."
      echo "  --bridge-config-file injects a local EMQX bridge snippet into the Helm release."
      echo "  Port-forwards are started automatically by default; use --no-start-port-forwards to disable that."
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

ENV_FILE="$(to_abs_path "$ENV_FILE")"
if [[ -n "$BRIDGE_CONFIG_FILE" ]]; then
  BRIDGE_CONFIG_FILE="$(to_abs_path "$BRIDGE_CONFIG_FILE")"
fi

ensure_cnpg_operator() {
  if kubectl get crd clusters.postgresql.cnpg.io >/dev/null 2>&1; then
    return 0
  fi

  echo "Installing CloudNativePG operator..."
  kubectl apply --server-side -f https://raw.githubusercontent.com/cloudnative-pg/cloudnative-pg/release-1.25/releases/cnpg-1.25.1.yaml
  kubectl -n cnpg-system rollout status deploy/cnpg-controller-manager --timeout=180s
}

minikube_has_local_registry_trust() {
  minikube -p "$MINIKUBE_PROFILE" ssh -- "ps -ef | grep dockerd | grep -F -- '$LOCAL_REGISTRY_PULL_HOST' >/dev/null" >/dev/null 2>&1
}

ensure_minikube_profile() {
  if ! minikube -p "$MINIKUBE_PROFILE" status >/dev/null 2>&1; then
    minikube -p "$MINIKUBE_PROFILE" start --driver=docker --insecure-registry="$LOCAL_REGISTRY_PULL_HOST"
  fi

  if ! minikube_has_local_registry_trust; then
    echo "Recreating minikube profile '$MINIKUBE_PROFILE' with trusted local registry $LOCAL_REGISTRY_PULL_HOST"
    minikube -p "$MINIKUBE_PROFILE" delete
    minikube -p "$MINIKUBE_PROFILE" start --driver=docker --insecure-registry="$LOCAL_REGISTRY_PULL_HOST"
  fi
}

ensure_local_registry() {
  if docker ps --format '{{.Names}}' | grep -qx "$LOCAL_REGISTRY_NAME"; then
    return 0
  fi

  if docker ps -a --format '{{.Names}}' | grep -qx "$LOCAL_REGISTRY_NAME"; then
    docker rm -f "$LOCAL_REGISTRY_NAME" >/dev/null 2>&1 || true
  fi

  docker run -d --restart=always -p 5000:5000 --name "$LOCAL_REGISTRY_NAME" registry:2 >/dev/null
}

build_and_push_image() {
  local image_name="$1"
  local dockerfile_path="$2"
  local build_context="$3"
  local full_image="$LOCAL_REGISTRY_PUSH_HOST/$image_name:$LOCAL_IMAGE_TAG"

  echo "Building $full_image"
  docker build -t "$full_image" -f "$dockerfile_path" "$build_context"
  docker push "$full_image" >/dev/null
}

build_core_images() {
  build_and_push_image "homenavi-frontend" "$HOMENAVI_ROOT/frontend/Dockerfile" "$HOMENAVI_ROOT/frontend"
  build_and_push_image "homenavi-api-gateway" "$HOMENAVI_ROOT/api-gateway/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-auth-service" "$HOMENAVI_ROOT/auth-service/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-user-service" "$HOMENAVI_ROOT/user-service/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-dashboard-service" "$HOMENAVI_ROOT/dashboard-service/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-email-service" "$HOMENAVI_ROOT/email-service/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-profile-picture-service" "$HOMENAVI_ROOT/profile-picture-service/Dockerfile" "$HOMENAVI_ROOT/profile-picture-service"
  build_and_push_image "homenavi-integration-proxy" "$HOMENAVI_ROOT/integration-proxy/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-echo-service" "$HOMENAVI_ROOT/echo-service/Dockerfile" "$HOMENAVI_ROOT/echo-service"
  build_and_push_image "homenavi-device-hub" "$HOMENAVI_ROOT/device-hub/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-entity-registry-service" "$HOMENAVI_ROOT/entity-registry-service/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-zigbee-adapter" "$HOMENAVI_ROOT/zigbee-adapter/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-thread-adapter" "$HOMENAVI_ROOT/thread-adapter/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-history-service" "$HOMENAVI_ROOT/history-service/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-automation-service" "$HOMENAVI_ROOT/automation-service/Dockerfile" "$HOMENAVI_ROOT"
  build_and_push_image "homenavi-weather-service" "$HOMENAVI_ROOT/weather-service/Dockerfile" "$HOMENAVI_ROOT"
}

build_marketplace_images() {
  build_and_push_image "homenavi-marketplace-api" "$MARKETPLACE_ROOT/api/Dockerfile" "$MARKETPLACE_ROOT/api"
  build_and_push_image "homenavi-marketplace-web" "$MARKETPLACE_ROOT/web/Dockerfile" "$MARKETPLACE_ROOT/web"
}

discover_bridge_files() {
  if [[ -n "$BRIDGE_CONFIG_FILE" ]]; then
    printf '%s\n' "$BRIDGE_CONFIG_FILE"
    return 0
  fi

  local bridge_dir="$HOMENAVI_ROOT/emqx/bridge.d"
  if [[ ! -d "$bridge_dir" ]]; then
    return 0
  fi

  find "$bridge_dir" -maxdepth 1 -type f -name '*.hocon' ! -name '*.example.hocon' | sort
}

write_bridge_values() {
  local values_file="$1"
  mapfile -t bridge_files < <(discover_bridge_files)

  if [[ ${#bridge_files[@]} -eq 0 ]]; then
    return 1
  fi

  {
    echo "services:"
    echo "  emqx:"
    echo "    bridgeConfigFiles:"
    for bridge_file in "${bridge_files[@]}"; do
      local bridge_name
      bridge_name="$(basename "$bridge_file")"
      echo "      ${bridge_name}: |"
      sed 's/^/        /' "$bridge_file"
    done
  } > "$values_file"

  echo "Injecting EMQX bridge snippets: ${bridge_files[*]}"
}

write_core_image_values() {
  local values_file="$1"

  cat > "$values_file" <<EOF
global:
  imagePullPolicy: IfNotPresent
services:
  frontend:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-frontend
      tag: ${LOCAL_IMAGE_TAG}
  api-gateway:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-api-gateway
      tag: ${LOCAL_IMAGE_TAG}
  auth-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-auth-service
      tag: ${LOCAL_IMAGE_TAG}
  user-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-user-service
      tag: ${LOCAL_IMAGE_TAG}
  dashboard-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-dashboard-service
      tag: ${LOCAL_IMAGE_TAG}
  email-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-email-service
      tag: ${LOCAL_IMAGE_TAG}
  profile-picture-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-profile-picture-service
      tag: ${LOCAL_IMAGE_TAG}
  integration-proxy:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-integration-proxy
      tag: ${LOCAL_IMAGE_TAG}
  echo-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-echo-service
      tag: ${LOCAL_IMAGE_TAG}
  device-hub:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-device-hub
      tag: ${LOCAL_IMAGE_TAG}
  entity-registry-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-entity-registry-service
      tag: ${LOCAL_IMAGE_TAG}
  zigbee-adapter:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-zigbee-adapter
      tag: ${LOCAL_IMAGE_TAG}
  thread-adapter:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-thread-adapter
      tag: ${LOCAL_IMAGE_TAG}
  history-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-history-service
      tag: ${LOCAL_IMAGE_TAG}
  automation-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-automation-service
      tag: ${LOCAL_IMAGE_TAG}
  weather-service:
    image:
      repository: ${LOCAL_REGISTRY_PULL_HOST}/homenavi-weather-service
      tag: ${LOCAL_IMAGE_TAG}
EOF
}

eval "$(minikube -p "$MINIKUBE_PROFILE" docker-env -u --shell bash)"

ensure_minikube_profile
ensure_local_registry

bash "$ENSURE_JWT_KEYS_SCRIPT" --dir /tmp/homenavi-keys

kubectl create namespace homenavi --dry-run=client -o yaml | kubectl apply -f -
if [[ "$DEPLOY_MARKETPLACE" == "true" ]]; then
  kubectl create namespace homenavi-marketplace --dry-run=client -o yaml | kubectl apply -f -
fi

ensure_cnpg_operator

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

echo "Building local core images with tag $LOCAL_IMAGE_TAG"
build_core_images

if [[ "$DEPLOY_MARKETPLACE" == "true" && "$SKIP_MARKETPLACE_BUILD" != "true" ]]; then
  echo "Building local marketplace images with tag $LOCAL_IMAGE_TAG"
  build_marketplace_images
fi

cd "$HOMENAVI_ROOT"
TMP_LOCAL_IMAGE_VALUES="$(mktemp)"
write_core_image_values "$TMP_LOCAL_IMAGE_VALUES"

HELM_ARGS=(
  upgrade --install homenavi ./helm/homenavi
  -n homenavi
  -f "$TMP_LOCAL_IMAGE_VALUES"
  --wait
  --timeout 15m
  --set-file jwt.privateKey=/tmp/homenavi-keys/jwt_private.pem
  --set-file jwt.publicKey=/tmp/homenavi-keys/jwt_public.pem
  --set global.imagePullPolicy=IfNotPresent
  --set persistentVolumeClaims.zigbee2mqtt-data.enabled=true
)

if [[ -f "$ENV_FILE" ]]; then
  HELM_ARGS+=(--set "global.envFromSecrets[0]=$RUNTIME_SECRET_NAME")
fi

TMP_BRIDGE_VALUES=""
cleanup() {
  if [[ -n "$TMP_BRIDGE_VALUES" && -f "$TMP_BRIDGE_VALUES" ]]; then
    rm -f "$TMP_BRIDGE_VALUES"
  fi
  if [[ -n "$TMP_LOCAL_IMAGE_VALUES" && -f "$TMP_LOCAL_IMAGE_VALUES" ]]; then
    rm -f "$TMP_LOCAL_IMAGE_VALUES"
  fi
}
trap cleanup EXIT

if [[ -n "$BRIDGE_CONFIG_FILE" && ! -f "$BRIDGE_CONFIG_FILE" ]]; then
  echo "Bridge config file not found: $BRIDGE_CONFIG_FILE"
  exit 1
fi

TMP_BRIDGE_VALUES="$(mktemp)"
if write_bridge_values "$TMP_BRIDGE_VALUES"; then
  HELM_ARGS+=(-f "$TMP_BRIDGE_VALUES")
else
  rm -f "$TMP_BRIDGE_VALUES"
  TMP_BRIDGE_VALUES=""
fi

if [[ -n "$BRIDGE_CONFIG_FILE" && -z "$TMP_BRIDGE_VALUES" ]]; then
  TMP_BRIDGE_VALUES="$(mktemp)"
  write_bridge_values "$TMP_BRIDGE_VALUES"
  HELM_ARGS+=(-f "$TMP_BRIDGE_VALUES")
fi

helm "${HELM_ARGS[@]}"

if [[ "$DEPLOY_MARKETPLACE" == "true" ]]; then
  cd "$MARKETPLACE_ROOT"
  helm upgrade --install homenavi-marketplace ./helm/homenavi-marketplace \
    -n homenavi-marketplace \
    --wait \
    --timeout 15m \
    --set api.image.repository=${LOCAL_REGISTRY_PULL_HOST}/homenavi-marketplace-api \
    --set api.image.tag=${LOCAL_IMAGE_TAG} \
    --set web.image.repository=${LOCAL_REGISTRY_PULL_HOST}/homenavi-marketplace-web \
    --set web.image.tag=${LOCAL_IMAGE_TAG} \
    --set imagePullPolicy=IfNotPresent
fi

echo ""
echo "Deployment summary:"
echo "  Image tag: ${LOCAL_IMAGE_TAG}"
kubectl -n homenavi get pods
if [[ "$DEPLOY_MARKETPLACE" == "true" ]]; then
  kubectl -n homenavi-marketplace get pods
fi

echo ""
echo "Windows browser access:"
FRONTEND_PORT="$(pick_first_free_port 50001)"
echo "  kubectl -n homenavi port-forward --address 0.0.0.0 svc/frontend ${FRONTEND_PORT}:80"
if [[ "$DEPLOY_MARKETPLACE" == "true" ]]; then
  MARKETPLACE_PORT="$(pick_first_free_port 50010)"
  echo "  kubectl -n homenavi-marketplace port-forward --address 0.0.0.0 svc/homenavi-marketplace ${MARKETPLACE_PORT}:80"
fi

if [[ "$START_PORT_FORWARDS" == "true" ]]; then
  FRONTEND_LOG="/tmp/homenavi-port-forward-frontend.log"
  MARKETPLACE_LOG="/tmp/homenavi-port-forward-marketplace.log"

  nohup kubectl -n homenavi port-forward --address 0.0.0.0 svc/frontend "${FRONTEND_PORT}:80" >"$FRONTEND_LOG" 2>&1 &
  FRONTEND_PF_PID=$!

  MARKETPLACE_PF_PID=""
  if [[ "$DEPLOY_MARKETPLACE" == "true" ]]; then
    nohup kubectl -n homenavi-marketplace port-forward --address 0.0.0.0 svc/homenavi-marketplace "${MARKETPLACE_PORT}:80" >"$MARKETPLACE_LOG" 2>&1 &
    MARKETPLACE_PF_PID=$!
  fi

  sleep 1

  echo ""
  echo "Auto-started port-forwards:"

  if kill -0 "$FRONTEND_PF_PID" >/dev/null 2>&1; then
    echo "  Frontend:    http://127.0.0.1:${FRONTEND_PORT} (pid=${FRONTEND_PF_PID}, log=${FRONTEND_LOG})"
  else
    echo "  Frontend port-forward failed. See ${FRONTEND_LOG}"
  fi

  if [[ "$DEPLOY_MARKETPLACE" == "true" ]]; then
    if kill -0 "$MARKETPLACE_PF_PID" >/dev/null 2>&1; then
      echo "  Marketplace: http://127.0.0.1:${MARKETPLACE_PORT} (pid=${MARKETPLACE_PF_PID}, log=${MARKETPLACE_LOG})"
    else
      echo "  Marketplace port-forward failed. See ${MARKETPLACE_LOG}"
    fi
  fi

  echo ""
  echo "To stop:"
  if [[ "$DEPLOY_MARKETPLACE" == "true" ]]; then
    echo "  kill ${FRONTEND_PF_PID} ${MARKETPLACE_PF_PID}"
  else
    echo "  kill ${FRONTEND_PF_PID}"
  fi
fi
