#!/bin/bash
# What this does:
# 1. Deletes any existing 'topology-test' Kind cluster.
# 2. Creates a new 3-node Kind cluster using deploy/kind/kind-config.yaml.
# 3. Deploys and patches the Metrics Server to allow insecure TLS (required for Kind).
# 4. Generates self-signed TLS certificates for the webhook.
# 5. Prepares the environment for building and deploying the operator.

set -euo pipefail

KIND_CLUSTER="desktop"

# Detect kind binary path
KIND_BIN="kind"
if ! command -v kind &> /dev/null; then
  if [ -f "$HOME/go/bin/kind" ]; then
    KIND_BIN="$HOME/go/bin/kind"
  elif [ -f "$HOME/go/bin/kind.exe" ]; then
    KIND_BIN="$HOME/go/bin/kind.exe"
  else
    # Fallback/guess for Windows environment in git bash / WSL
    USERPROFILE_WIN=$(cmd.exe /c "echo %USERPROFILE%" 2>/dev/null | tr -d '\r')
    if [ -n "${USERPROFILE_WIN}" ]; then
      WSL_USERPROFILE=$(echo "${USERPROFILE_WIN}" | sed -e 's/\\/\//g' -e 's/^C:/\/mnt\/c/')
      GITBASH_USERPROFILE=$(echo "${USERPROFILE_WIN}" | sed -e 's/\\/\//g' -e 's/^C:/\/c/')
      if [ -f "${WSL_USERPROFILE}/go/bin/kind.exe" ]; then
        KIND_BIN="${WSL_USERPROFILE}/go/bin/kind.exe"
      elif [ -f "${GITBASH_USERPROFILE}/go/bin/kind.exe" ]; then
        KIND_BIN="${GITBASH_USERPROFILE}/go/bin/kind.exe"
      fi
    fi
    if [ "${KIND_BIN}" = "kind" ]; then
      if [ -f "/c/Users/$(whoami)/go/bin/kind" ]; then
        KIND_BIN="/c/Users/$(whoami)/go/bin/kind"
      elif [ -f "/c/Users/$(whoami)/go/bin/kind.exe" ]; then
        KIND_BIN="/c/Users/$(whoami)/go/bin/kind.exe"
      elif [ -f "/mnt/c/Users/$(whoami)/go/bin/kind.exe" ]; then
        KIND_BIN="/mnt/c/Users/$(whoami)/go/bin/kind.exe"
      fi
    fi
  fi
fi

echo "Using kind binary: ${KIND_BIN}"

echo "=== 1. Checking if cluster already exists ==="
if "${KIND_BIN}" get clusters | grep -q "^${KIND_CLUSTER}$"; then
  echo "Cluster '${KIND_CLUSTER}' is already running. Skipping creation and using existing cluster."
else
  echo "=== 2. Creating Kind Cluster ==="
  "${KIND_BIN}" create cluster --config deploy/kind/kind-config.yaml --name "${KIND_CLUSTER}"
fi

echo "=== 2. Labeling worker nodes with topology ==="
kubectl label node desktop-worker topology.kubernetes.io/zone=us-east-1a topology.kubernetes.io/rack=rack-a --overwrite
kubectl label node desktop-worker2 topology.kubernetes.io/zone=us-east-1b topology.kubernetes.io/rack=rack-b --overwrite
kubectl label node desktop-worker3 topology.kubernetes.io/zone=us-east-1a topology.kubernetes.io/rack=rack-a --overwrite

echo "=== 3. Deploying Metrics Server ==="
kubectl apply -f https://github.com/kubernetes-sigs/metrics-server/releases/latest/download/components.yaml

echo "=== Patching Metrics Server for Kind (Insecure Kubelet TLS) ==="
kubectl patch deployment metrics-server -n kube-system --type='json' -p='[
  {"op": "add", "path": "/spec/template/spec/containers/0/args/-", "value": "--kubelet-insecure-tls"}
]'

echo "Waiting for Metrics Server deployment to roll out..."
kubectl rollout status deployment/metrics-server -n kube-system --timeout=120s

echo "=== 4. Generating Webhook TLS Certificates ==="
bash deploy/certs/generate-certs.sh

echo "=== Setup Complete! ==="
echo "You can verify metrics are active with 'kubectl top nodes' in a minute."

