#!/bin/sh
# Purpose: Create the two kind clusters used for KubeSift development and apply their sample workloads.
# Used by: A developer or agent preparing this machine. Documented in docs/engineering/local-development.html.
# Responsibilities: Create dev-local-a and dev-local-b when absent, then apply one small workload stack to each.
# Important: Does not delete clusters, install tools, or change kubeconfig current-context when the previous context still exists. Secret values are fake local placeholders. Every kubectl call names its context.
# ponytail: one control-plane node and pause containers. Add a worker or a serving image when a demo needs scheduling or HTTP.
set -eu

NODE='kindest/node:v1.37.0@sha256:a1ed56cfb0e7b93589bdf97c8cd566405a265939e3620fc4f5de89adff580ae5'

if ! command -v docker >/dev/null 2>&1; then
  echo "Docker is not installed. Install it, then rerun scripts/dev-clusters.sh." >&2
  exit 1
fi
if ! docker info >/dev/null 2>&1; then
  echo "Docker is not running. Start it, then rerun scripts/dev-clusters.sh." >&2
  exit 1
fi
if ! command -v kind >/dev/null 2>&1; then
  echo "kind is not installed. Install it, then rerun scripts/dev-clusters.sh." >&2
  exit 1
fi
if ! command -v kubectl >/dev/null 2>&1; then
  echo "kubectl is not installed. Install it, then rerun scripts/dev-clusters.sh." >&2
  exit 1
fi

previous=$(kubectl config current-context 2>/dev/null || true)

restore_context() {
  if [ -n "$previous" ] && kubectl config get-contexts -o name | grep -qx "$previous"; then
    kubectl config use-context "$previous" >/dev/null
  fi
}

apply_stack() {
  context=$1
  ns=$2
  app=$3
  worker=$4
  settings=$5
  reference=$6
  credential=$7
  token=$8
  agent=$9
  shift 9
  public_route=$1
  worker_route=$2
  public_host=$3
  worker_host=$4

  kubectl apply --context "$context" -f - <<EOF
apiVersion: v1
kind: Namespace
metadata:
  name: ${ns}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${app}
  namespace: ${ns}
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ${app}
  template:
    metadata:
      labels:
        app: ${app}
    spec:
      containers:
      - name: app
        image: registry.k8s.io/pause:3.10
        ports:
        - name: http
          containerPort: 8080
        resources:
          requests:
            cpu: 1m
            memory: 4Mi
          limits:
            cpu: 10m
            memory: 8Mi
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: ${worker}
  namespace: ${ns}
spec:
  replicas: 2
  selector:
    matchLabels:
      app: ${worker}
  template:
    metadata:
      labels:
        app: ${worker}
    spec:
      containers:
      - name: worker
        image: registry.k8s.io/pause:3.10
        resources:
          requests:
            cpu: 1m
            memory: 4Mi
          limits:
            cpu: 10m
            memory: 8Mi
---
apiVersion: v1
kind: Service
metadata:
  name: ${app}
  namespace: ${ns}
spec:
  selector:
    app: ${app}
  ports:
  - name: http
    port: 80
    targetPort: http
---
apiVersion: v1
kind: Service
metadata:
  name: ${worker}
  namespace: ${ns}
spec:
  selector:
    app: ${worker}
  ports:
  - name: http
    port: 80
    targetPort: 8080
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${settings}
  namespace: ${ns}
data:
  log-level: info
  listen: "8080"
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: ${reference}
  namespace: ${ns}
data:
  owner: kubesift-local
  notes: local sample data
---
apiVersion: v1
kind: Secret
metadata:
  name: ${credential}
  namespace: ${ns}
type: Opaque
stringData:
  username: local
  password: local-dev-only
---
apiVersion: v1
kind: Secret
metadata:
  name: ${token}
  namespace: ${ns}
type: Opaque
stringData:
  token: local-dev-only
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: ${agent}
  namespace: ${ns}
spec:
  selector:
    matchLabels:
      app: ${agent}
  template:
    metadata:
      labels:
        app: ${agent}
    spec:
      containers:
      - name: agent
        image: registry.k8s.io/pause:3.10
        resources:
          requests:
            cpu: 1m
            memory: 4Mi
          limits:
            cpu: 10m
            memory: 8Mi
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${public_route}
  namespace: ${ns}
spec:
  rules:
  - host: ${public_host}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: ${app}
            port:
              name: http
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: ${worker_route}
  namespace: ${ns}
spec:
  rules:
  - host: ${worker_host}
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: ${worker}
            port:
              name: http
EOF
  kubectl --context "$context" -n "$ns" rollout status "deployment/$app" --timeout=180s
  kubectl --context "$context" -n "$ns" rollout status "deployment/$worker" --timeout=180s
  kubectl --context "$context" -n "$ns" rollout status "daemonset/$agent" --timeout=180s
  echo "Applied sample workloads to $context / $ns."
}

ensure_cluster() {
  name=$1
  if ! kind get clusters | grep -qx "$name"; then
    kind create cluster --name "$name" --image "$NODE"
    restore_context
    echo "Created kind cluster $name."
  else
    echo "Reusing kind cluster $name."
  fi
}

ensure_cluster dev-local-a
ensure_cluster dev-local-b
apply_stack kind-dev-local-a shop web-frontend cart-api web-settings catalog-copy web-tls cart-db node-log-agent storefront cart storefront.local cart.local
apply_stack kind-dev-local-b billing invoice-api ledger-worker invoice-settings tax-rates billing-db payment-token metric-agent invoices ledger invoices.local ledger.local
restore_context

cat <<'EOF'
Development contexts in the default kubeconfig:
  kind-dev-local-a  namespace shop: web-frontend, cart-api, services, configmaps, secrets, node-log-agent, ingresses storefront and cart
  kind-dev-local-b  namespace billing: invoice-api, ledger-worker, services, configmaps, secrets, metric-agent, ingresses invoices and ledger
Each cluster has one control-plane node. Load local configuration in the app with a blank kubeconfig path.
EOF
