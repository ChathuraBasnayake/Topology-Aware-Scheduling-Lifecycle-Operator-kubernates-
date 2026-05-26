#!/bin/bash
# What this does:
# 1. Creates the 'topology-system' namespace.
# 2. Deploys the Webhook TLS secret using the generated certificates.
# 3. Injects the CA bundle into deploy/webhook/webhook-config.yaml and applies it.
# 4. Deploys the Webhook service and deployment.
# 5. Deploys the Custom Controller RBAC and deployment.
# 6. Deploys the Custom Scheduler ConfigMap, RBAC, and deployment.

set -euo pipefail

NAMESPACE="topology-system"

echo "=== 1. Creating Namespace ==="
kubectl create namespace "${NAMESPACE}" --dry-run=client -o yaml | kubectl apply -f -

echo "=== 2. Creating Webhook TLS Secret ==="
if [ -f "deploy/certs/tls.crt" ] && [ -f "deploy/certs/tls.key" ]; then
  kubectl create secret tls webhook-tls \
    --cert=deploy/certs/tls.crt \
    --key=deploy/certs/tls.key \
    -n "${NAMESPACE}" \
    --dry-run=client -o yaml | kubectl apply -f -
else
  echo "❌ Error: Webhook certificates not found at deploy/certs/. Run scripts/setup-cluster.sh first."
  exit 1
fi

echo "=== 3. Deploying Webhook Components ==="
# Get base64 encoded CA cert (compatible with different base64 command styles)
if [[ "$OSTYPE" == "darwin"* ]]; then
  CA_BUNDLE=$(cat deploy/certs/ca.crt | base64)
else
  CA_BUNDLE=$(cat deploy/certs/ca.crt | base64 -w 0)
fi

# Apply mutated webhook config with injected CA bundle
sed "s/CA_BUNDLE_PLACEHOLDER/${CA_BUNDLE}/g" deploy/webhook/webhook-config.yaml | kubectl apply -f -

kubectl apply -f deploy/webhook/service.yaml
kubectl apply -f deploy/webhook/deployment.yaml

echo "Waiting for Webhook to be ready..."
kubectl rollout status deployment/topology-webhook -n "${NAMESPACE}" --timeout=60s

echo "=== Webhook Deployment Done ==="

echo "=== 4. Deploying Custom Controller ==="
kubectl apply -f deploy/controller/rbac.yaml
kubectl apply -f deploy/controller/deployment.yaml

echo "Waiting for Custom Controller to be ready..."
kubectl rollout status deployment/topology-controller -n "${NAMESPACE}" --timeout=60s

echo "=== Custom Controller Deployment Done ==="

