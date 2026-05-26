# Topology-Aware Scheduling & Lifecycle Operator

A production-grade Kubernetes operator and custom scheduler framework written in Go. The system optimizes pod placement by intercepting pod creation at Admission Control, calculating real-time node health via a Custom Controller (monitoring metrics from the Metrics Server), and scheduling pods onto the most optimal nodes using a Custom Scheduler Plugin.

---

## System Architecture

The operator is divided into three cooperative components running inside the `topology-system` namespace:

```
Developer (kubectl apply)
       │
       ▼
   API Server
       │
       ├─► Mutating Webhook (Synchronous Interception)
       │       │ - Detects topology-aware policy annotations
       │       │ - Injects schedulerName & rack nodeAffinity rules
       │       ▼
       ├─► etcd Database (Stores mutated Pending Pod)
       │
       ├─► Custom Controller (Asynchronous Background Loop)
       │       │ - Watches Pod allocations & Node status via Informers
       │       │ - Queries live CPU/Memory utilization from Metrics Server
       │       │ - Patches Node annotations with composite Health Scores
       │       ▼
       └─► Custom Scheduler (Framework Plugin)
               │ - Filters out nodes with low health (< 30) or rack mismatches
               │ - Scores remaining nodes based on CPU headroom & pod density
               └─► Binds Pod to the optimal Node
```

---

## Core Components

### 1. Mutating Admission Webhook (`cmd/webhook/`)
- Intercepts Pod creation requests before they are persisted in etcd.
- Inspects annotations such as `topology-aware.io/policy` and `topology-aware.io/target-rack`.
- Mutates the pod spec using RFC 6902 JSON Patches to inject the custom `schedulerName` and configure Node Affinities.
- Operates under a Fail-Open policy to guarantee cluster resilience even if the webhook goes offline.

### 2. Custom Controller (`cmd/controller/`)
- Built using the Kubernetes Informer Pattern to read data from a low-latency, thread-safe local cache rather than querying the API server directly.
- Periodically queries the cluster's Metrics Server for live OS-level resource usage.
- Computes a composite health score (0-100) for each node using the formula:
  $$\text{Health} = (\text{CPU Headroom} \times 45\%) + (\text{Memory Headroom} \times 45\%) + (\text{NodeReady} \times 10\%)$$
- Updates the Node objects in etcd using StrategicMergePatches to record utilization telemetry.

### 3. Custom Scheduler Plugin (`cmd/scheduler/`)
- Compiled directly into the official `kube-scheduler` command registry.
- Implements two extension points of the Kubernetes Scheduling Framework:
  - **Filter Stage:** Discards nodes with a health score < 30, or nodes whose topological rack label does not match a pod's requested `target-rack`.
  - **Score Stage:** Ranks eligible nodes based on resource headroom and pod density:
    $$\text{Score} = (\text{HealthScore} \times 40\%) + (\text{CPU Headroom} \times 40\%) + (\text{PodDensity} \times 20\%)$$

---

## Getting Started

### Prerequisites
- Docker Desktop or Kind installed
- Go (v1.26+)
- kubectl CLI
- openssl (for TLS certificate generation)

### 1. Set Up Local Cluster & Telemetry
Run the cluster setup script. This creates a Kind cluster named `desktop`, labels the nodes with topology keys, deploys the Metrics Server, patches it for insecure TLS (required for Kind), and generates self-signed TLS certificates for the webhook:
```bash
bash scripts/setup-cluster.sh
```

### 2. Build and Load Docker Images
Compile the binaries and build the distroless container images locally, then load them into the Kind cluster:
```bash
# Build Webhook image
docker build -t topology-operator/webhook:latest -f docker/Dockerfile.webhook .
kind load docker-image topology-operator/webhook:latest --name desktop

# Build Controller image
docker build -t topology-operator/controller:latest -f docker/Dockerfile.controller .
kind load docker-image topology-operator/controller:latest --name desktop

# Build Scheduler image
docker build -t topology-operator/scheduler:latest -f docker/Dockerfile.scheduler .
kind load docker-image topology-operator/scheduler:latest --name desktop
```

### 3. Deploy the Operator
Apply the configurations, service accounts, RBAC bindings, and deployments to the cluster:
```bash
bash scripts/deploy-all.sh
```

Ensure all pods are running successfully:
```bash
kubectl get pods -n topology-system
```

---

## Testing & Verification

### Running Unit Tests
Execute the test suites for the webhook, controller, and scheduler packages:
```bash
go test -v ./pkg/...
```

### Deploying a Test Workload
Deploy a pod with a `low-latency` scheduling policy targeting `rack-a`:
```bash
kubectl apply -f deploy/test/pod-low-latency.yaml
```

Verify that:
1. **Webhook Mutation:** The pod was routed to our scheduler and injected with a node affinity requirement:
   ```bash
   kubectl get pod test-low-latency -o yaml
   # Verify: 'schedulerName: topology-aware-scheduler' and affinity rules are present.
   ```
2. **Telemetry Calculation:** The custom controller successfully calculated node health and annotated the nodes:
   ```bash
   kubectl get nodes -o jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.metadata.annotations.topology-aware\.io/health-score}{"\n"}{end}'
   ```
3. **Scheduler Binding:** The custom scheduler successfully placed the pod on a node matching the target rack.

---

## Project Structure
```
├── cmd/
│   ├── webhook/           # Webhook main entrypoint
│   ├── controller/        # Custom Controller main entrypoint
│   └── scheduler/         # Custom Scheduler main entrypoint
├── pkg/
│   ├── webhook/           # Webhook mutation logic & policy definitions
│   ├── controller/        # Informer setups & telemetry calculation
│   └── scheduler/         # Custom Scheduling framework plugin & test cases
├── deploy/
│   ├── certs/             # Certificate scripts & configurations
│   ├── webhook/           # Webhook Deployment, Service, and WebhookConfig
│   ├── controller/        # Controller Deployment & RBAC
│   ├── scheduler/         # Scheduler Deployment, RBAC, and ConfigMap
│   └── test/              # Test Pod manifests
└── scripts/               # Automation scripts for setups & deployments
```
