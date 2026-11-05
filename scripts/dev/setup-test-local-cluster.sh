#!/bin/sh
# Purpose: Keep the shared local review clusters present for the app and for agents.
# Used by: A developer or agent resuming work on a machine.
# Responsibilities: Create the three kind clusters when absent, then apply their committed manifests.
# Important: Do not install tools, delete clusters, create kubesift-b, or use this as validation teardown.
set -eu

ROOT=$(CDPATH= cd -- "$(dirname "$0")/../.." && pwd)
CONFIG="$ROOT/scripts/dev/kubesift-dev/kind-config.yaml"
NODE='kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5'

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is not installed. Install it, then rerun scripts/dev/setup-test-local-cluster.sh." >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Docker is not running. Start it, then rerun scripts/dev/setup-test-local-cluster.sh." >&2
  exit 1
fi
if ! command -v kind >/dev/null 2>&1; then
  echo "kind is not installed. Install it, then rerun scripts/dev/setup-test-local-cluster.sh." >&2
  exit 1
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is not installed. Install it, then rerun scripts/dev/setup-test-local-cluster.sh." >&2
  exit 1
fi

ensure_cluster() {
  name=$1
  manifest=$2
  context=kind-$name
  if ! kind get clusters | grep -qx "$name"; then
    previous=$(kubectl config current-context 2>/dev/null || true)
    kind create cluster --name "$name" --image "$NODE" --config "$CONFIG"
    if [ -n "$previous" ]; then
      kubectl config use-context "$previous" >/dev/null
    fi
    echo "Created kind cluster $name."
  else
    echo "Reusing kind cluster $name."
  fi
  kubectl apply -f "$manifest" --context "$context"
  echo "Applied $(basename "$manifest") to context $context."
}

ensure_cluster dev-local "$ROOT/scripts/dev/kubesift-dev/dev-bootstrap.yaml"
ensure_cluster dev-healthy "$ROOT/scripts/dev/kubesift-dev/dev-healthy.yaml"
ensure_cluster dev-broken "$ROOT/scripts/dev/kubesift-dev/dev-broken.yaml"

cat <<'EOF'
Review contexts in the default kubeconfig:
  kind-dev-local    payments/checkout-api crashes; payments-db and app-stg/web-ui run
  kind-dev-healthy  shop/storefront runs
  kind-dev-broken   incidents/checkout-api crashes; incidents/missing-image cannot pull
Load local configuration in the app with a blank kubeconfig path. Leave these clusters in place.
Validation fixtures remain scripts/local-clusters.sh up and must not replace these clusters.
EOF
