// Package webhook provides a type-safe builder for RFC 6902 JSON Patch operations.
package webhook

import (
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
)

// PatchOperation represents a single RFC 6902 JSON Patch operation.
type PatchOperation struct {
	Op    string      `json:"op"`              // "add", "replace", "remove"
	Path  string      `json:"path"`            // JSON Pointer path
	Value interface{} `json:"value,omitempty"` // New value (nil for "remove")
}

// addSchedulerName creates a patch that sets the Pod's schedulerName field
// so it gets routed to our custom scheduler instead of the default one.
func addSchedulerName() PatchOperation {
	return PatchOperation{
		Op:    "add",
		Path:  "/spec/schedulerName",
		Value: "topology-aware-scheduler",
	}
}

// addLabel creates a patch that adds a label to the Pod's metadata.
// If labels don't exist yet, we need to create the whole map first.
func addLabel(key, value string) PatchOperation {
	// Encode "/" in the key as "~1" per RFC 6901
	encodedKey := strings.ReplaceAll(key, "/", "~1")
	return PatchOperation{
		Op:    "add",
		Path:  fmt.Sprintf("/metadata/labels/%s", encodedKey),
		Value: value,
	}
}

// ensureLabelsExist creates the /metadata/labels map if it doesn't exist.
// This MUST be called before addLabel if the Pod has no labels.
func ensureLabelsExist(pod *corev1.Pod) []PatchOperation {
	if pod.Labels == nil {
		return []PatchOperation{{
			Op:    "add",
			Path:  "/metadata/labels",
			Value: map[string]string{},
		}}
	}
	return nil
}
