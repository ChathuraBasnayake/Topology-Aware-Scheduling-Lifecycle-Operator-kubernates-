package webhook

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// Helper to build a mock AdmissionReview request
func buildAdmissionReviewRequest(pod *corev1.Pod) ([]byte, error) {
	podBytes, err := json.Marshal(pod)
	if err != nil {
		return nil, err
	}

	ar := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			UID: "test-uid-12345",
			Object: runtime.RawExtension{
				Raw: podBytes,
			},
			Operation: admissionv1.Create,
		},
	}

	return json.Marshal(ar)
}

func TestMutate_NoAnnotation(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}

	body, err := buildAdmissionReviewRequest(pod)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleMutate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK, got %v", resp.StatusCode)
	}

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(w.Body).Decode(&review); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if review.Response == nil {
		t.Fatal("Response is nil")
	}

	if !review.Response.Allowed {
		t.Error("Expected Allowed to be true")
	}

	if len(review.Response.Patch) > 0 {
		t.Errorf("Expected no patches, got %s", string(review.Response.Patch))
	}
}

func TestMutate_LowLatencyPolicy(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				PolicyAnnotation:     PolicyLowLatency,
				TargetRackAnnotation: "rack-b",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}

	body, err := buildAdmissionReviewRequest(pod)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleMutate(w, req)

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(w.Body).Decode(&review); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if review.Response == nil {
		t.Fatal("Response is nil")
	}

	if !review.Response.Allowed {
		t.Error("Expected Allowed to be true")
	}

	if len(review.Response.Patch) == 0 {
		t.Fatal("Expected patches, got none")
	}

	// Verify patch contains low-latency rack-b requirement, scheduler name and mutated label
	patchStr := string(review.Response.Patch)
	if !strings.Contains(patchStr, "rack-b") {
		t.Errorf("Expected patch to contain target rack 'rack-b', got %s", patchStr)
	}
	if !strings.Contains(patchStr, "topology-aware-scheduler") {
		t.Errorf("Expected patch to contain schedulerName, got %s", patchStr)
	}
	if !strings.Contains(patchStr, "topology-aware.io~1mutated") {
		t.Errorf("Expected patch to contain mutated label, got %s", patchStr)
	}
}

func TestMutate_SpreadPolicy(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				PolicyAnnotation: PolicySpread,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}

	body, err := buildAdmissionReviewRequest(pod)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleMutate(w, req)

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(w.Body).Decode(&review); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if review.Response == nil {
		t.Fatal("Response is nil")
	}

	if !review.Response.Allowed {
		t.Error("Expected Allowed to be true")
	}

	if len(review.Response.Patch) == 0 {
		t.Fatal("Expected patches, got none")
	}

	patchStr := string(review.Response.Patch)
	if !strings.Contains(patchStr, "topologySpreadConstraints") {
		t.Errorf("Expected patch to contain topologySpreadConstraints, got %s", patchStr)
	}
	if !strings.Contains(patchStr, "topology.kubernetes.io/zone") {
		t.Errorf("Expected patch to contain zone constraint, got %s", patchStr)
	}
}

func TestMutate_ColocatePolicy(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				PolicyAnnotation:   PolicyColocate,
				AppGroupAnnotation: "test-app-group",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}

	body, err := buildAdmissionReviewRequest(pod)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleMutate(w, req)

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(w.Body).Decode(&review); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if review.Response == nil {
		t.Fatal("Response is nil")
	}

	if !review.Response.Allowed {
		t.Error("Expected Allowed to be true")
	}

	if len(review.Response.Patch) == 0 {
		t.Fatal("Expected patches, got none")
	}

	patchStr := string(review.Response.Patch)
	if !strings.Contains(patchStr, "podAffinity") {
		t.Errorf("Expected patch to contain podAffinity, got %s", patchStr)
	}
	if !strings.Contains(patchStr, "test-app-group") {
		t.Errorf("Expected patch to contain target app group 'test-app-group', got %s", patchStr)
	}
}

func TestMutate_UnknownPolicy(t *testing.T) {
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Annotations: map[string]string{
				PolicyAnnotation: "invalid-policy",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}

	body, err := buildAdmissionReviewRequest(pod)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleMutate(w, req)

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(w.Body).Decode(&review); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if review.Response == nil {
		t.Fatal("Response is nil")
	}

	if !review.Response.Allowed {
		t.Error("Expected Allowed to be true")
	}

	if len(review.Response.Patch) > 0 {
		t.Errorf("Expected no patches for unknown policy, got %s", string(review.Response.Patch))
	}
}

func TestMutate_MalformedBody(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/mutate", strings.NewReader("garbage bytes"))
	w := httptest.NewRecorder()

	HandleMutate(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status Bad Request (400), got %v", resp.StatusCode)
	}
}

func TestMutate_Idempotency(t *testing.T) {
	// First mutation is tested in other tests. Here we verify that if we pass a pod
	// that already has labels, they are preserved or appended to.
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pod",
			Namespace: "default",
			Labels: map[string]string{
				"existing-key": "existing-value",
			},
			Annotations: map[string]string{
				PolicyAnnotation: PolicyLowLatency,
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx",
				},
			},
		},
	}

	body, err := buildAdmissionReviewRequest(pod)
	if err != nil {
		t.Fatalf("Failed to build request: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	HandleMutate(w, req)

	var review admissionv1.AdmissionReview
	if err := json.NewDecoder(w.Body).Decode(&review); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if review.Response == nil {
		t.Fatal("Response is nil")
	}

	if len(review.Response.Patch) == 0 {
		t.Fatal("Expected patches, got none")
	}

	patchStr := string(review.Response.Patch)
	if strings.Contains(patchStr, "existing-key") {
		t.Errorf("Patch should not recreate or overwrite existing labels map: %s", patchStr)
	}
}
