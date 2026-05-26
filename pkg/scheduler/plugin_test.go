package scheduler

import (
	"context"
	"fmt"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/kube-scheduler/framework"
)

// Mock implementations for testing the framework Interfaces.

type mockPodInfo struct {
	framework.PodInfo
}

type mockNodeInfo struct {
	framework.NodeInfo
	node *corev1.Node
	pods []framework.PodInfo
}

func (m *mockNodeInfo) Node() *corev1.Node {
	return m.node
}

func (m *mockNodeInfo) GetPods() []framework.PodInfo {
	return m.pods
}

type mockNodeInfoLister struct {
	framework.NodeInfoLister
	nodeInfos []framework.NodeInfo
}

func (m *mockNodeInfoLister) Get(nodeName string) (framework.NodeInfo, error) {
	for _, ni := range m.nodeInfos {
		if ni.Node() != nil && ni.Node().Name == nodeName {
			return ni, nil
		}
	}
	return nil, fmt.Errorf("node %q not found", nodeName)
}

type mockSharedLister struct {
	framework.SharedLister
	nodeInfos []framework.NodeInfo
}

func (m *mockSharedLister) NodeInfos() framework.NodeInfoLister {
	return &mockNodeInfoLister{nodeInfos: m.nodeInfos}
}

type mockHandle struct {
	framework.Handle
	sharedLister framework.SharedLister
}

func (m *mockHandle) SnapshotSharedLister() framework.SharedLister {
	return m.sharedLister
}

func TestFilter(t *testing.T) {
	tests := []struct {
		name       string
		pod        *corev1.Pod
		node       *corev1.Node
		wantStatus *framework.Status
	}{
		{
			name: "Healthy node, no policy",
			pod:  &corev1.Pod{},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-1",
					Annotations: map[string]string{"topology-aware.io/health-score": "80"},
				},
			},
			wantStatus: nil,
		},
		{
			name: "Unhealthy node, filtered out",
			pod:  &corev1.Pod{},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-1",
					Annotations: map[string]string{"topology-aware.io/health-score": "25"},
				},
			},
			wantStatus: framework.NewStatus(framework.Unschedulable, "node health score 25 is below threshold (30)"),
		},
		{
			name: "Low latency policy, correct rack",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"topology-aware.io/policy":      "low-latency",
						"topology-aware.io/target-rack": "rack-a",
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-1",
					Annotations: map[string]string{"topology-aware.io/rack": "rack-a"},
				},
			},
			wantStatus: nil,
		},
		{
			name: "Low latency policy, rack mismatch",
			pod: &corev1.Pod{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"topology-aware.io/policy":      "low-latency",
						"topology-aware.io/target-rack": "rack-a",
					},
				},
			},
			node: &corev1.Node{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "node-1",
					Annotations: map[string]string{"topology-aware.io/rack": "rack-b"},
				},
			},
			wantStatus: framework.NewStatus(framework.Unschedulable, `node rack "rack-b" does not match target rack "rack-a"`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin := &TopologyAwarePlacement{}
			nodeInfo := &mockNodeInfo{node: tt.node}

			status := plugin.Filter(context.Background(), nil, tt.pod, nodeInfo)
			if tt.wantStatus == nil {
				if status != nil {
					t.Errorf("expected status nil, got: %v", status)
				}
			} else {
				if status == nil {
					t.Errorf("expected status %v, got nil", tt.wantStatus)
				} else if status.Code() != tt.wantStatus.Code() || status.Message() != tt.wantStatus.Message() {
					t.Errorf("expected status %v, got: %v", tt.wantStatus, status)
				}
			}
		})
	}
}

func TestScore(t *testing.T) {
	node1 := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node-1",
			Annotations: map[string]string{
				"topology-aware.io/health-score":    "90",
				"topology-aware.io/cpu-utilization": "20",
			},
		},
	}

	nodeInfo1 := &mockNodeInfo{
		node: node1,
		pods: []framework.PodInfo{
			&mockPodInfo{},
			&mockPodInfo{},
		},
	}

	sharedLister := &mockSharedLister{
		nodeInfos: []framework.NodeInfo{nodeInfo1},
	}
	handle := &mockHandle{
		sharedLister: sharedLister,
	}

	plugin := &TopologyAwarePlacement{handle: handle}
	pod := &corev1.Pod{}

	score, status := plugin.Score(context.Background(), nil, pod, nodeInfo1)
	if !status.IsSuccess() {
		t.Fatalf("expected score success, got: %v", status)
	}

	// Calculate expected score:
	// healthScore = 90
	// cpuUtil = 20 -> cpuHeadroom = 80
	// podCount = 2 -> podDensityScore = 100 - 2*10 = 80
	// score = (90*40 + 80*40 + 80*20) / 100 = (3600 + 3200 + 1600) / 100 = 8400 / 100 = 84
	var expectedScore int64 = 84
	if score != expectedScore {
		t.Errorf("expected score %d, got: %d", expectedScore, score)
	}
}
