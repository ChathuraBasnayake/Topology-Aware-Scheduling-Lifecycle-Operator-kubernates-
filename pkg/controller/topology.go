// Package controller implements the live telemetry aggregator for topology metrics.
package controller

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

// LiveTopologyAggregator calculates topology metrics from real-time telemetry.
type LiveTopologyAggregator struct{}

func NewLiveTopologyAggregator() *LiveTopologyAggregator {
	return &LiveTopologyAggregator{}
}

// BuildLiveAnnotations takes the Node allocations and Metrics Server cAdvisor usage
// data, and computes live resource utilization and health score annotations.
func (a *LiveTopologyAggregator) BuildLiveAnnotations(
	node *corev1.Node,
	nodeMetrics *metricsv1beta1.NodeMetrics,
) map[string]string {
	allocatableCPU := node.Status.Allocatable.Cpu().MilliValue() // in millicores
	allocatableMem := node.Status.Allocatable.Memory().Value()   // in bytes

	currentCPUUsage := nodeMetrics.Usage.Cpu().MilliValue()
	currentMemUsage := nodeMetrics.Usage.Memory().Value()

	var cpuPercent int
	if allocatableCPU > 0 {
		cpuPercent = int((currentCPUUsage * 100) / allocatableCPU)
	} else {
		cpuPercent = 100
	}

	var memPercent int
	if allocatableMem > 0 {
		memPercent = int((currentMemUsage * 100) / allocatableMem)
	} else {
		memPercent = 100
	}

	if cpuPercent > 100 {
		cpuPercent = 100
	}
	if memPercent > 100 {
		memPercent = 100
	}

	liveCPUHeadroom := 100 - cpuPercent
	liveMemHeadroom := 100 - memPercent

	nodeReady := 0
	for _, condition := range node.Status.Conditions {
		if condition.Type == corev1.NodeReady && condition.Status == corev1.ConditionTrue {
			nodeReady = 100
			break
		}
	}

	liveHealthScore := (liveCPUHeadroom*45 + liveMemHeadroom*45 + nodeReady*10) / 100

	if liveHealthScore < 0 {
		liveHealthScore = 0
	}
	if liveHealthScore > 100 {
		liveHealthScore = 100
	}

	return map[string]string{
		"topology-aware.io/cpu-utilization":    strconv.Itoa(cpuPercent),
		"topology-aware.io/memory-utilization": strconv.Itoa(memPercent),
		"topology-aware.io/health-score":       strconv.Itoa(liveHealthScore),
		"topology-aware.io/rack":               node.Labels["topology.kubernetes.io/rack"],
		"topology-aware.io/zone":               node.Labels["topology.kubernetes.io/zone"],
	}
}
