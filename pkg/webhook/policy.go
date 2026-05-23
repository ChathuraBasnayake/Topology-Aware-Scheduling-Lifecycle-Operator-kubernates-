// Package webhook maps the topology-aware.io/policy annotation on a Pod to concrete
// Kubernetes scheduling constructs (nodeAffinity, topologySpreadConstraints,
// podAffinity).
package webhook

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/klog/v2"
)

const (
	// Annotation keys — these are what developers put on their Pods
	PolicyAnnotation     = "topology-aware.io/policy"
	TargetRackAnnotation = "topology-aware.io/target-rack"
	AppGroupAnnotation   = "topology-aware.io/app-group"

	// Annotation values
	PolicyLowLatency = "low-latency"
	PolicySpread     = "spread"
	PolicyColocate   = "colocate"
)

// EvaluatePolicy examines the Pod's annotations and returns the appropriate
// JSON Patch operations. Returns nil patches if no policy annotation is found.
func EvaluatePolicy(pod *corev1.Pod) ([]PatchOperation, error) {
	policy, exists := pod.Annotations[PolicyAnnotation]
	if !exists {
		// No policy annotation — pass through without mutation
		return nil, nil
	}

	klog.Infof("Evaluating topology policy '%s' for pod %s/%s", policy, pod.Namespace, pod.Name)

	var patches []PatchOperation

	// Always ensure labels map exists before adding to it
	patches = append(patches, ensureLabelsExist(pod)...)

	switch policy {
	case PolicyLowLatency:
		p, err := buildLowLatencyPatch(pod)
		if err != nil {
			return nil, err
		}
		patches = append(patches, p...)

	case PolicySpread:
		patches = append(patches, buildSpreadPatch(pod)...)

	case PolicyColocate:
		patches = append(patches, buildColocatePatch(pod)...)

	default:
		klog.Warningf("Unknown topology policy '%s' for pod %s/%s, skipping mutation",
			policy, pod.Namespace, pod.Name)
		return nil, nil
	}

	// Always inject our custom scheduler name and a "mutated" label
	patches = append(patches, addSchedulerName())
	patches = append(patches, addLabel("topology-aware.io/mutated", "true"))

	return patches, nil
}

// buildLowLatencyPatch creates nodeAffinity rules that require the Pod to be
// scheduled on a specific network rack for minimal latency.
//
// The target rack is read from the annotation: topology-aware.io/target-rack
// If not specified, defaults to "rack-a".
func buildLowLatencyPatch(pod *corev1.Pod) ([]PatchOperation, error) {
	targetRack := pod.Annotations[TargetRackAnnotation]
	if targetRack == "" {
		targetRack = "rack-a" // sensible default for testing
	}

	// Build the nodeAffinity structure that will be injected into the Pod spec.
	// This tells the scheduler: "ONLY place this Pod on nodes labeled rack=<targetRack>"
	affinity := corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{
			RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
				NodeSelectorTerms: []corev1.NodeSelectorTerm{
					{
						MatchExpressions: []corev1.NodeSelectorRequirement{
							{
								Key:      "topology.kubernetes.io/rack",
								Operator: corev1.NodeSelectorOpIn,
								Values:   []string{targetRack},
							},
						},
					},
				},
			},
		},
	}

	return []PatchOperation{
		{
			Op:    "add",
			Path:  "/spec/affinity",
			Value: affinity,
		},
	}, nil
}

// buildSpreadPatch creates topologySpreadConstraints that distribute Pods
// evenly across availability zones.
func buildSpreadPatch(pod *corev1.Pod) []PatchOperation {
	constraints := []corev1.TopologySpreadConstraint{
		{
			MaxSkew:           1,
			TopologyKey:       "topology.kubernetes.io/zone",
			WhenUnsatisfiable: corev1.DoNotSchedule,
			LabelSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"topology-aware.io/mutated": "true",
				},
			},
		},
	}

	return []PatchOperation{
		{
			Op:    "add",
			Path:  "/spec/topologySpreadConstraints",
			Value: constraints,
		},
	}
}

// buildColocatePatch creates podAffinity rules that prefer placing this Pod
// on the same rack as other Pods in the same application group.
func buildColocatePatch(pod *corev1.Pod) []PatchOperation {
	appGroup := pod.Annotations[AppGroupAnnotation]
	if appGroup == "" {
		appGroup = "default-group"
	}

	affinity := corev1.Affinity{
		PodAffinity: &corev1.PodAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						TopologyKey: "topology.kubernetes.io/rack",
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"topology-aware.io/app-group": appGroup,
							},
						},
					},
				},
			},
		},
	}

	return []PatchOperation{
		{
			Op:    "add",
			Path:  "/spec/affinity",
			Value: affinity,
		},
		addLabel("topology-aware.io/app-group", appGroup),
	}
}
