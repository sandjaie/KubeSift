#!/bin/bash
set -e

echo "🚀 Setting up KubeSift Local Development Environment..."

# 1. Check/Install Prerequisites
if ! command -v docker &> /dev/null; then
    echo "❌ Docker not found. Installing via Homebrew..."
    brew install --cask docker
fi

if ! command -v kind &> /dev/null; then
    echo "❌ Kind not found. Installing via Homebrew..."
    brew install kind
fi

# 2. Create Working Directory
mkdir -p kubesift-dev
cd kubesift-dev

# 3. Create Kind Configuration File
echo "📝 Writing kind-config.yaml..."
cat << 'yaml' > kind-config.yaml
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
- role: worker
yaml

# 4. Create App Manifests File
echo "📝 Writing dev-bootstrap.yaml..."
cat << 'yaml' > dev-bootstrap.yaml
---
apiVersion: v1
kind: Namespace
metadata:
  name: payments
---
apiVersion: v1
kind: Namespace
metadata:
  name: app-stg
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: payments-db
  namespace: payments
spec:
  replicas: 1
  selector:
    matchLabels:
      app: payments-db
  template:
    metadata:
      labels:
        app: payments-db
    spec:
      containers:
      - name: postgres
        image: postgres:15-alpine
        env:
        - name: POSTGRES_PASSWORD
          value: "supersecret"
        resources:
          limits:
            cpu: "100m"
            memory: "128Mi"
          requests:
            cpu: "50m"
            memory: "64Mi"
---
apiVersion: v1
kind: Service
metadata:
  name: payments-db
  namespace: payments
spec:
  ports:
  - port: 5432
  selector:
    app: payments-db
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
  namespace: payments
spec:
  replicas: 1
  selector:
    matchLabels:
      app: checkout-api
  template:
    metadata:
      labels:
        app: checkout-api
    spec:
      containers:
      - name: checkout-api
        image: alpine:latest
        command: ["/bin/sh", "-c"]
        args:
        - |
          echo "[$(date +'%Y-%m-%d %H:%M:%S')] Starting checkout-api"
          echo "[$(date +'%Y-%m-%d %H:%M:%S')] Loading configuration"
          sleep 1
          echo "[$(date +'%Y-%m-%d %H:%M:%S')] ERROR required setting DATABASE_URL is missing"
          exit 1
        env:
        - name: DATABASE_HOST
          value: "db.payments.svc.cluster.local"
        - name: DATABASE_PORT
          value: "5432"
        resources:
          limits:
            cpu: "50m"
            memory: "64Mi"
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: web-ui
  namespace: app-stg
spec:
  replicas: 1
  selector:
    matchLabels:
      app: web-ui
  template:
    metadata:
      labels:
        app: web-ui
    spec:
      containers:
      - name: nginx
        image: nginx:alpine
        resources:
          limits:
            cpu: "50m"
            memory: "64Mi"
---
apiVersion: v1
kind: Service
metadata:
  name: web-ui-service
  namespace: app-stg
spec:
  ports:
  - port: 80
    targetPort: 80
  selector:
    app: web-ui
yaml

# 5. Spin Up Cluster
echo "🏗️ Provisioning Kind cluster 'dev-local'..."
kind create cluster --name dev-local --config kind-config.yaml

# 6. Apply Manifests
echo "📦 Injecting application workspaces..."
kubectl apply -f dev-bootstrap.yaml --context kind-dev-local

echo "✅ Environment Ready!"
echo "🔍 Run 'kubectl get pods -n payments --context kind-dev-local' to check the crash state."
