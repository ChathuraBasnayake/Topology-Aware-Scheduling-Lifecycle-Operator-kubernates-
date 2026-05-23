# ============================================================================
# Topology-Aware Scheduling & Lifecycle Operator — Build System
# ============================================================================

REGISTRY     ?= topology-operator
WEBHOOK_IMG  := $(REGISTRY)/webhook:latest
CTRL_IMG     := $(REGISTRY)/controller:latest
SCHED_IMG    := $(REGISTRY)/scheduler:latest
KIND_CLUSTER := topology-test

# ---- Build ----
.PHONY: build-webhook build-controller build-scheduler build-all

build-webhook:
	CGO_ENABLED=0 GOOS=linux go build -o bin/webhook ./cmd/webhook/

build-controller:
	CGO_ENABLED=0 GOOS=linux go build -o bin/controller ./cmd/controller/

build-scheduler:
	CGO_ENABLED=0 GOOS=linux go build -o bin/scheduler ./cmd/scheduler/

build-all: build-webhook build-controller build-scheduler

# ---- Docker ----
.PHONY: docker-build docker-push kind-load

docker-build:
	docker build -t $(WEBHOOK_IMG)  -f docker/Dockerfile.webhook .
	docker build -t $(CTRL_IMG)     -f docker/Dockerfile.controller .
	docker build -t $(SCHED_IMG)    -f docker/Dockerfile.scheduler .

kind-load:
	kind load docker-image $(WEBHOOK_IMG)  --name $(KIND_CLUSTER)
	kind load docker-image $(CTRL_IMG)     --name $(KIND_CLUSTER)
	kind load docker-image $(SCHED_IMG)    --name $(KIND_CLUSTER)

# ---- Test ----
.PHONY: test test-cover

test:
	go test ./pkg/... -v -count=1

test-cover:
	go test ./pkg/... -v -coverprofile=coverage.out
	go tool cover -html=coverage.out -o coverage.html

# ---- Cluster ----
.PHONY: kind-create kind-delete setup deploy teardown

kind-create:
	kind create cluster --config deploy/kind/kind-config.yaml --name $(KIND_CLUSTER)

kind-delete:
	kind delete cluster --name $(KIND_CLUSTER)

setup: kind-create docker-build kind-load deploy

deploy:
	bash scripts/deploy-all.sh

teardown:
	bash scripts/teardown.sh

# ---- Certs ----
.PHONY: generate-certs

generate-certs:
	bash deploy/certs/generate-certs.sh
