// Package scheduler implements the Custom Scheduler Plugin for Topology-Aware Scheduling.
package scheduler

import (
	"context"
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kube-scheduler/framework"
)

// TopologyAwarePlacement is a scheduler plugin that scores and filters nodes
// based on live health metrics, CPU headroom, and topological rack alignment.
type TopologyAwarePlacement struct {
	handle framework.Handle
}

// Name is the name of the plugin.
const Name = "TopologyAwarePlacement"

// Name returns the name of the plugin.
func (pl *TopologyAwarePlacement) Name() string {
	return Name
}

// New initializes a new TopologyAwarePlacement plugin.
func New(ctx context.Context, configuration runtime.Object, fh framework.Handle) (framework.Plugin, error) {
	return &TopologyAwarePlacement{
		handle: fh,
	}, nil
}

// Filter checks if a Node is eligible to run the Pod.
// It filters out nodes that have low health (< 30) or mismatch the target rack.
func (pl *TopologyAwarePlacement) Filter(
	ctx context.Context,
	state framework.CycleState,
	pod *corev1.Pod,
	nodeInfo framework.NodeInfo,
) *framework.Status {
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found in nodeInfo")
	}

	// 1. Filter by Health Score (reject nodes with health < 30)
	healthScoreStr, ok := node.Annotations["topology-aware.io/health-score"]
	if ok {
		healthScore, err := strconv.Atoi(healthScoreStr)
		if err == nil && healthScore < 30 {
			klog.V(3).InfoS("Node filtered: health score below threshold", "node", node.Name, "health", healthScore)
			return framework.NewStatus(
				framework.Unschedulable,
				fmt.Sprintf("node health score %d is below threshold (30)", healthScore),
			)
		}
	}

	// 2. Filter by Rack alignment (if low-latency policy is requested)
	policy := pod.Annotations["topology-aware.io/policy"]
	if policy == "low-latency" {
		targetRack := pod.Annotations["topology-aware.io/target-rack"]
		if targetRack != "" {
			nodeRack := node.Annotations["topology-aware.io/rack"]
			// Fallback to label if controller annotation isn't calculated yet
			if nodeRack == "" {
				nodeRack = node.Labels["topology.kubernetes.io/rack"]
			}

			if nodeRack != targetRack {
				klog.V(3).InfoS("Node filtered: rack mismatch for low-latency pod",
					"node", node.Name, "nodeRack", nodeRack, "targetRack", targetRack)
				return framework.NewStatus(
					framework.Unschedulable,
					fmt.Sprintf("node rack %q does not match target rack %q", nodeRack, targetRack),
				)
			}
		}
	}

	return nil
}

// Score ranks eligible nodes.
// Total Score (0-100) = (HealthScore * 40%) + (CPUHeadroom * 40%) + (PodDensityScore * 20%)
func (pl *TopologyAwarePlacement) Score(
	ctx context.Context,
	state framework.CycleState,
	pod *corev1.Pod,
	nodeInfo framework.NodeInfo,
) (int64, *framework.Status) {
	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, "node not found in nodeInfo")
	}

	// 1. Get health-score annotation (default 50)
	var healthScore int64 = 50
	if valStr, ok := node.Annotations["topology-aware.io/health-score"]; ok {
		if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
			healthScore = val
		}
	}

	// 2. Get CPU utilization to compute CPU Headroom (default 0% utilization -> 100% headroom)
	var cpuUtil int64 = 0
	if valStr, ok := node.Annotations["topology-aware.io/cpu-utilization"]; ok {
		if val, err := strconv.ParseInt(valStr, 10, 64); err == nil {
			cpuUtil = val
		}
	}
	cpuHeadroom := 100 - cpuUtil
	if cpuHeadroom < 0 {
		cpuHeadroom = 0
	}

	// 3. Compute Pod density score based on snapshot pods on this node (lower density -> higher score)
	podCount := int64(len(nodeInfo.GetPods()))
	podDensityScore := 100 - (podCount * 10)
	if podDensityScore < 0 {
		podDensityScore = 0
	}

	// Compute composite score: health=40%, cpuHeadroom=40%, podDensity=20%
	score := (healthScore * 40) + (cpuHeadroom * 40) + (podDensityScore * 20)
	score = score / 100

	klog.V(4).InfoS("Scored node for pod", "pod", pod.Name, "node", node.Name, "score", score,
		"health", healthScore, "headroom", cpuHeadroom, "densityScore", podDensityScore)

	return score, nil
}

// ScoreExtensions returns the ScoreExtensions interface (nil because no normalization is needed).
func (pl *TopologyAwarePlacement) ScoreExtensions() framework.ScoreExtensions {
	return nil
}

// Ensure the plugin implements the correct interfaces at compile-time.
var _ framework.FilterPlugin = &TopologyAwarePlacement{}
var _ framework.ScorePlugin = &TopologyAwarePlacement{}
