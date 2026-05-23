# Topology-Aware Scheduling & Lifecycle Operator

A Kubernetes operator written in Go that intercepts Pod creation at **Admission Control**, maintains cluster topology state via a **Custom Controller** with **live telemetry from the Metrics Server**, and schedules Pods onto optimal nodes using a **Custom Scheduler Plugin**.

## Component Overview
1. **Mutating Admission Webhook**: Intercepts Pod admission requests, injects topology nodeAffinity policies, and routes them to our custom scheduler.
2. **Custom Controller**: Periodically queries node metrics from `metrics.k8s.io` (provided by Metrics Server) and updates Node annotations with health scores, utilization scores, and zone/rack information.
3. **Custom Scheduler Plugin**: A Kubernetes scheduler plugin that filters and scores nodes based on zone/rack topology, hardware capabilities, and real-time CPU/memory utilization.
