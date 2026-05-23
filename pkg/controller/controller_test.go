package controller

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metricsv1beta1 "k8s.io/metrics/pkg/apis/metrics/v1beta1"
)

func TestLiveTopologyAggregator_BuildLiveAnnotations(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
			Labels: map[string]string{
				"topology.kubernetes.io/rack": "rack-a",
				"topology.kubernetes.io/zone": "us-east-1a",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("4"),      // 4000m
				corev1.ResourceMemory: resource.MustParse("16Gi"),   // 16 * 1024 * 1024 * 1024
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionTrue,
				},
			},
		},
	}

	nodeMetrics := &metricsv1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Usage: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),     // 1000m (25% utilization)
			corev1.ResourceMemory: resource.MustParse("4Gi"),    // 4Gi (25% utilization)
		},
	}

	aggregator := NewLiveTopologyAggregator()
	annotations := aggregator.BuildLiveAnnotations(node, nodeMetrics)

	// Expectations:
	// CPU utilization: 25% (1000m / 4000m) -> 25
	// Mem utilization: 25% (4Gi / 16Gi) -> 25
	// NodeReady: True -> 100
	// HealthScore: ( (100 - 25)*45 + (100 - 25)*45 + 100*10 ) / 100 = ( 75*45 + 75*45 + 1000 ) / 100
	//            = ( 3375 + 3375 + 1000 ) / 100 = 7750 / 100 = 77
	
	if annotations["topology-aware.io/cpu-utilization"] != "25" {
		t.Errorf("Expected CPU utilization to be '25', got '%s'", annotations["topology-aware.io/cpu-utilization"])
	}

	if annotations["topology-aware.io/memory-utilization"] != "25" {
		t.Errorf("Expected memory utilization to be '25', got '%s'", annotations["topology-aware.io/memory-utilization"])
	}

	if annotations["topology-aware.io/health-score"] != "77" {
		t.Errorf("Expected health score to be '77', got '%s'", annotations["topology-aware.io/health-score"])
	}

	if annotations["topology-aware.io/rack"] != "rack-a" {
		t.Errorf("Expected rack annotation to be 'rack-a', got '%s'", annotations["topology-aware.io/rack"])
	}

	if annotations["topology-aware.io/zone"] != "us-east-1a" {
		t.Errorf("Expected zone annotation to be 'us-east-1a', got '%s'", annotations["topology-aware.io/zone"])
	}
}

func TestLiveTopologyAggregator_NodeNotReady(t *testing.T) {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
			Labels: map[string]string{
				"topology.kubernetes.io/rack": "rack-a",
				"topology.kubernetes.io/zone": "us-east-1a",
			},
		},
		Status: corev1.NodeStatus{
			Allocatable: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:   corev1.NodeReady,
					Status: corev1.ConditionFalse,
				},
			},
		},
	}

	nodeMetrics := &metricsv1beta1.NodeMetrics{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-node-1",
		},
		Usage: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),  // 50%
			corev1.ResourceMemory: resource.MustParse("4Gi"), // 50%
		},
	}

	aggregator := NewLiveTopologyAggregator()
	annotations := aggregator.BuildLiveAnnotations(node, nodeMetrics)

	// Expectations:
	// CPU utilization: 50%
	// Mem utilization: 50%
	// NodeReady: False -> 0
	// HealthScore: ( (100 - 50)*45 + (100 - 50)*45 + 0*10 ) / 100 = ( 50*45 + 50*45 + 0 ) / 100
	//            = ( 2250 + 2250 ) / 100 = 4500 / 100 = 45

	if annotations["topology-aware.io/health-score"] != "45" {
		t.Errorf("Expected health score to be '45', got '%s'", annotations["topology-aware.io/health-score"])
	}
}

func TestAnnotationsChanged(t *testing.T) {
	oldAnn := map[string]string{
		"topology-aware.io/health-score": "50",
	}

	newAnnIdentical := map[string]string{
		"topology-aware.io/health-score": "50",
	}

	newAnnChanged := map[string]string{
		"topology-aware.io/health-score": "60",
	}

	newAnnExtra := map[string]string{
		"topology-aware.io/health-score": "50",
		"topology-aware.io/new-key":      "val",
	}

	if annotationsChanged(oldAnn, newAnnIdentical) {
		t.Error("Expected annotationsChanged to return false for identical annotations")
	}

	if !annotationsChanged(oldAnn, newAnnChanged) {
		t.Error("Expected annotationsChanged to return true for changed value")
	}

	if !annotationsChanged(oldAnn, newAnnExtra) {
		t.Error("Expected annotationsChanged to return true for extra annotation key")
	}
}
