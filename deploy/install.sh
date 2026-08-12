#!/usr/bin/env bash
#
# install.sh — install the whole Olympus on-premise platform into a
# Kubernetes cluster with one command.
#
# Works against any Kubernetes cluster (kind, minikube, Docker Desktop,
# k3s, RKE, GKE, EKS, AKS, ...). It will:
#   1. sanity-check tooling and the target cluster,
#   2. ask you to confirm the cluster (it never installs without consent),
#   3. build all service images and either load them into a local cluster
#      (kind/minikube/Docker Desktop) or push them to a registry,
#   4. generate secrets once and store them locally for reuse,
#   5. `helm install` the umbrella chart and wait for readiness.
#
# Usage:
#   ./install.sh [flags]
#
# Flags:
#   -n, --namespace <ns>     Kubernetes namespace (default: olympus)
#   -r, --release <name>     Helm release name (default: olympus)
#   -c, --context <ctx>      kubectl context to target (default: current)
#       --registry <reg>     Registry to push built images to, e.g.
#                            ghcr.io/acme. If omitted and the cluster is not
#                            a local one (kind/minikube/docker), the script
#                            asks how to get images in.
#       --image-prefix <p>   Pull pre-built images from this prefix instead of
#                            building (e.g. ghcr.io/acme/olympus). Skips build.
#       --tag <tag>          Image tag (default: latest)
#       --values <file>      Extra Helm values file(s), layered on top
#       --set 'a=b'          Extra --set passthrough to helm (repeatable)
#       --ingress <host>     Enable ingress on this host for the console
#       --wait-timeout <d>   helm --wait timeout (default: 10m)
#       --yes                Skip the confirmation prompt (dangerous)
#       --skip-build         Don't build or load images; assume they are
#                            already available to the cluster
#       --dry-run            Render manifests and exit without installing
#   -h, --help               Show this help
#
# Examples:
#   ./install.sh                                 # local cluster, confirm first
#   ./install.sh --registry ghcr.io/acme         # push images, install
#   ./install.sh --image-prefix ghcr.io/acme/olympus --skip-build
#   ./install.sh --context kind-olympus --ingress console.olympus.example.com
#
set -euo pipefail

# --- configuration -----------------------------------------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
CHART_DIR="$SCRIPT_DIR/helm/olympus"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
STATE_DIR="${OLYMPUS_STATE_DIR:-$HOME/.olympus}"

NAMESPACE="olympus"
RELEASE="olympus"
CONTEXT=""
REGISTRY=""
IMAGE_PREFIX=""
TAG="latest"
EXTRA_VALUES=()
EXTRA_SET=()
INGRESS_HOST=""
WAIT_TIMEOUT="10m"
YES=0
SKIP_BUILD=0
DRY_RUN=0

SERVICES=(themis amphora paramdora hephaestus orpheus clio mneme iris prometheus console)

log()  { printf '\033[1;34m[olympus]\033[0m %s\n' "$*"; }
err()  { printf '\033[1;31m[olympus]\033[0m %s\n' "$*" >&2; }
die()  { err "$*"; exit 1; }

usage() { sed -n '2,40p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'; exit 0; }

# --- argument parsing --------------------------------------------------------
while [[ $# -gt 0 ]]; do
  case "$1" in
    -h|--help) usage ;;
    -n|--namespace)   NAMESPACE="${2:?}"; shift 2 ;;
    -r|--release)     RELEASE="${2:?}"; shift 2 ;;
    -c|--context)     CONTEXT="${2:?}"; shift 2 ;;
    --registry)       REGISTRY="${2:?}"; shift 2 ;;
    --image-prefix)   IMAGE_PREFIX="${2:?}"; shift 2 ;;
    --tag)            TAG="${2:?}"; shift 2 ;;
    --values)         EXTRA_VALUES+=("${2:?}"); shift 2 ;;
    --set)            EXTRA_SET+=("${2:?}"); shift 2 ;;
    --ingress)        INGRESS_HOST="${2:?}"; shift 2 ;;
    --wait-timeout)   WAIT_TIMEOUT="${2:?}"; shift 2 ;;
    --yes)            YES=1; shift ;;
    --skip-build)     SKIP_BUILD=1; shift ;;
    --dry-run)        DRY_RUN=1; shift ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
done

# --- tooling checks ----------------------------------------------------------
for tool in kubectl helm docker; do
  command -v "$tool" >/dev/null 2>&1 || die "missing required tool: $tool"
done
[[ -d "$CHART_DIR" ]] || die "chart directory not found: $CHART_DIR"

# --- target cluster ----------------------------------------------------------
if [[ -n "$CONTEXT" ]]; then
  kubectl config use-context "$CONTEXT" >/dev/null 2>&1 \
    || die "cannot switch to context '$CONTEXT'"
fi
CONTEXT="$(kubectl config current-context 2>/dev/null || true)"
if [[ -z "$CONTEXT" ]]; then
  die "no kubectl context selected (and none given with --context)."
fi

CLUSTER_INFO="$(kubectl cluster-info --request-timeout=5s 2>/dev/null | sed -n '1,2p' || true)"
if [[ -z "$CLUSTER_INFO" ]]; then
  die "cannot reach the cluster for context '$CONTEXT' (kubectl cluster-info failed)."
fi
log "target cluster:"
printf '%s\n' "$CLUSTER_INFO" | sed 's/^/    /'

# Refuse to touch obviously-non-target clusters unless forced. This protects
# production/staging clusters from accidental installs.
if [[ "$CONTEXT" =~ (^|/)(prod|production|staging|st-eks)[-/] ]] && [[ "$YES" != 1 ]]; then
  die "context '$CONTEXT' looks like a production/staging cluster. \
Re-run with --context pointing at your target, or --yes to override."
fi

if [[ "$YES" != 1 ]]; then
  read -r -p "[olympus] Install into namespace '$NAMESPACE' on cluster '$CONTEXT'? [y/N] " ans
  [[ "$ans" =~ ^[Yy] ]] || die "aborted."
fi

# --- secrets (generated once, reused on later runs) --------------------------
mkdir -p "$STATE_DIR"
SEED_FILE="$STATE_DIR/secrets-$RELEASE.env"
JWT=""; PG_PASS=""; MINIO_AK=""; MINIO_SK=""
if [[ -f "$SEED_FILE" ]]; then
  # shellcheck disable=SC1090
  source "$SEED_FILE"
  log "reusing existing secrets from $SEED_FILE"
else
  JWT="$(openssl rand -hex 32)"
  PG_PASS="$(openssl rand -hex 16)"
  MINIO_AK="olympus$(openssl rand -hex 8)"
  MINIO_SK="$(openssl rand -hex 24)"
  umask 177
  cat > "$SEED_FILE" <<EOF
JWT="$JWT"
PG_PASS="$PG_PASS"
MINIO_AK="$MINIO_AK"
MINIO_SK="$MINIO_SK"
EOF
  chmod 600 "$SEED_FILE"
  log "generated new secrets (kept in $SEED_FILE)"
fi

# --- image strategy ----------------------------------------------------------
: "${JWT:?}" : "${PG_PASS:?}" : "${MINIO_AK:?}" : "${MINIO_SK:?}"
IMAGE_SETS=()
PULL_POLICY="IfNotPresent"

image_ref() { # svc -> local image name
  case "$1" in
    amphora)   echo "olympus/olympus" ;;
    *)         echo "olympus/$1" ;;
  esac
}

# Every service image is built from the repo root: the console Dockerfile
# COPYs both web/ and console/, and the service Dockerfiles COPY their own
# dir plus the local authz replace target.
build_args() { # svc -> docker build flags
  echo "-f" "$REPO_ROOT/$1/Dockerfile" "$REPO_ROOT"
}

detect_local_cluster() {
  if [[ "$CONTEXT" == kind-* ]]; then echo "kind"; return; fi
  if [[ "$CONTEXT" == minikube* ]]; then echo "minikube"; return; fi
  if kubectl config view -o jsonpath='{.current-context}' 2>/dev/null | grep -qi docker-desktop; then
    echo "docker-desktop"; return
  fi
  echo ""
}

if [[ "$SKIP_BUILD" == 1 ]]; then
  if [[ -n "$IMAGE_PREFIX" ]]; then
    for svc in "${SERVICES[@]}"; do
      IMAGE_SETS+=("--set" "$svc.image.repository=$IMAGE_PREFIX/$(basename "$(image_ref "$svc")")")
      IMAGE_SETS+=("--set" "$svc.image.tag=$TAG")
    done
  fi
  log "skipping image build (--skip-build); images must already be available to the cluster"
elif [[ -n "$REGISTRY" ]]; then
  log "building and pushing images to $REGISTRY ..."
  for svc in "${SERVICES[@]}"; do
    local_img="$(image_ref "$svc")"
    remote_img="$REGISTRY/${local_img}"
    log "  build+push ${local_img}:$TAG -> ${remote_img}:$TAG"
    docker build -q -t "$remote_img:$TAG" $(build_args "$svc") \
      || die "docker build failed for $svc"
    docker push -q "$remote_img:$TAG" || die "docker push failed for ${remote_img}:$TAG"
    IMAGE_SETS+=("--set" "$svc.image.repository=${remote_img}")
    IMAGE_SETS+=("--set" "$svc.image.tag=$TAG")
  done
  IMAGE_SETS+=("--set" "prometheus.imagePrefix=$REGISTRY/olympus/prometheus-fn")
  PULL_POLICY="Always"
else
  LOCAL="$(detect_local_cluster)"
  if [[ -n "$LOCAL" ]]; then
    log "detected local cluster: $LOCAL"
    for svc in "${SERVICES[@]}"; do
      local_img="$(image_ref "$svc")"
      log "  build ${local_img}:$TAG"
      docker build -q -t "$local_img:$TAG" $(build_args "$svc") \
        || die "docker build failed for $svc"
      case "$LOCAL" in
        kind)            kind load docker-image "$local_img:$TAG" >/dev/null ;;
        minikube)        minikube image load "$local_img:$TAG" >/dev/null ;;
        docker-desktop)  : ;; # images already visible to the cluster runtime
      esac
      IMAGE_SETS+=("--set" "$svc.image.tag=$TAG")
    done
  else
    err "no --registry given and the target does not look like a local cluster."
    err "Install images via one of:"
    err "  ./install.sh --registry ghcr.io/<you>/olympus"
    err "  ./install.sh --image-prefix ghcr.io/<you>/olympus --skip-build"
    exit 1
  fi
fi

# --- build the final values --------------------------------------------------
# We pass secrets + images via --set so the chart's values.yaml stays pristine.
HELM_SET=(
  --set "global.jwtSecret=$JWT"
  --set "global.postgres.password=$PG_PASS"
  --set "global.minio.accessKey=$MINIO_AK"
  --set "global.minio.secretKey=$MINIO_SK"
  --set "global.minio.bucket=olympus"
  "${IMAGE_SETS[@]}"
)

if [[ -n "$INGRESS_HOST" ]]; then
  HELM_SET+=(--set "ingress.enabled=true" --set "ingress.hosts[0].host=$INGRESS_HOST")
fi

HELM_ARGS=(upgrade --install "$RELEASE" "$CHART_DIR" -n "$NAMESPACE" --create-namespace
  --wait --timeout "$WAIT_TIMEOUT")
for v in "${EXTRA_VALUES[@]}"; do HELM_ARGS+=(--values "$v"); done
for s in "${HELM_SET[@]}"; do HELM_ARGS+=("$s"); done
for s in "${EXTRA_SET[@]}"; do HELM_ARGS+=(--set "$s"); done

# --- dry-run ----------------------------------------------------------------
if [[ "$DRY_RUN" == 1 ]]; then
  log "dry-run: rendering manifests (no install)"
  if ! helm template "$RELEASE" "$CHART_DIR" -n "$NAMESPACE" "${HELM_SET[@]}" "${EXTRA_SET[@]/#/--set }" >/dev/null 2>&1; then
    helm template "$RELEASE" "$CHART_DIR" -n "$NAMESPACE" "${HELM_SET[@]}" "${EXTRA_SET[@]/#/--set }"
    die "helm template failed"
  fi
  log "rendered ok."
  exit 0
fi

# --- install ----------------------------------------------------------------
log "installing release '$RELEASE' into namespace '$NAMESPACE' (this can take a few minutes)..."
# shellcheck disable=SC2086
helm "${HELM_ARGS[@]}"

log "waiting for workloads to become ready..."
kubectl -n "$NAMESPACE" rollout status --timeout="${WAIT_TIMEOUT}" \
  deployment/"$RELEASE"-postgres 2>/dev/null || true
kubectl -n "$NAMESPACE" wait --for=condition=available --timeout="${WAIT_TIMEOUT}" \
  --selector app.kubernetes.io/instance="$RELEASE" --all deployments >/dev/null 2>&1 || \
  err "some deployments are not ready yet; check: kubectl -n $NAMESPACE get pods"

# --- done -------------------------------------------------------------------
log "install complete."
cat <<EOF

  Console:  kubectl -n $NAMESPACE port-forward svc/$RELEASE-console 8090:8090
            -> open http://localhost:8090
EOF
if [[ -n "$INGRESS_HOST" ]]; then
  printf '  Ingress: https://%s\n' "$INGRESS_HOST"
fi
printf '%s\n' \
  "  Next: create an IAM user + access key in Themis, attach a policy," \
  "        then sign in from the console header with that key."
