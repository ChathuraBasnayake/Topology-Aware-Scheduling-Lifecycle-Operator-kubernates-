// Package webhook receives AdmissionReview JSON from the API server, extracts the Pod,
// checks for our topology annotation, builds a JSON Patch if needed,
// and returns the patched AdmissionReview response.
package webhook

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/klog/v2"
)

// codecs is the universal deserializer that can decode any Kubernetes API object.
// We use it to parse the incoming AdmissionReview from raw bytes.
var codecs = serializer.NewCodecFactory(runtime.NewScheme())

func HandleMutate(w http.ResponseWriter, r *http.Request) {
	// ---- Step 1: Read the HTTP body ----
	body, err := io.ReadAll(r.Body)
	if err != nil {
		klog.Errorf("Failed to read request body: %v", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	// ---- Step 2: Decode into AdmissionReview ----
	// The API server sends an AdmissionReview wrapper containing:
	//   .Request.UID         — unique ID for this review (must echo back)
	//   .Request.Object.Raw  — the raw JSON of the Pod being created
	//   .Request.Operation   — CREATE, UPDATE, DELETE, etc.
	var admissionReview admissionv1.AdmissionReview
	if err := json.Unmarshal(body, &admissionReview); err != nil {
		klog.Errorf("Failed to unmarshal AdmissionReview: %v", err)
		http.Error(w, "failed to unmarshal", http.StatusBadRequest)
		return
	}

	// ---- Step 3: Extract the Pod from the request ----
	var pod corev1.Pod
	if err := json.Unmarshal(admissionReview.Request.Object.Raw, &pod); err != nil {
		klog.Errorf("Failed to unmarshal Pod: %v", err)
		respondWithError(w, admissionReview, fmt.Sprintf("failed to unmarshal pod: %v", err))
		return
	}

	klog.Infof("Received admission request for Pod: %s/%s", pod.Namespace, pod.Name)

	// ---- Step 4: Evaluate topology policy ----
	// Check if the Pod has our custom annotation.
	// If not, allow it through without modification (fail-open).
	patches, err := EvaluatePolicy(&pod)
	if err != nil {
		klog.Errorf("Policy evaluation failed: %v", err)
		respondWithError(w, admissionReview, err.Error())
		return
	}

	// ---- Step 5: Build the response ----
	response := &admissionv1.AdmissionResponse{
		UID:     admissionReview.Request.UID,  // MUST echo back the UID
		Allowed: true,                          // Always allow (we mutate, not validate)
	}

	if len(patches) > 0 {
		// Marshal the patch array to JSON, then the K8s API expects it as raw bytes
		patchBytes, err := json.Marshal(patches)
		if err != nil {
			klog.Errorf("Failed to marshal patches: %v", err)
			respondWithError(w, admissionReview, "failed to marshal patches")
			return
		}
		patchType := admissionv1.PatchTypeJSONPatch
		response.PatchType = &patchType
		response.Patch = patchBytes
		klog.Infof("Applying %d patch operations to Pod %s/%s", len(patches), pod.Namespace, pod.Name)
	} else {
		klog.Infof("No topology policy found for Pod %s/%s, passing through", pod.Namespace, pod.Name)
	}

	// ---- Step 6: Send the response ----
	admissionReview.Response = response
	admissionReview.Response.UID = admissionReview.Request.UID

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(admissionReview)
}

// respondWithError sends a denied AdmissionReview response.
func respondWithError(w http.ResponseWriter, ar admissionv1.AdmissionReview, message string) {
	ar.Response = &admissionv1.AdmissionResponse{
		UID:     ar.Request.UID,
		Allowed: false,
		Result: &metav1.Status{
			Message: message,
		},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ar)
}
