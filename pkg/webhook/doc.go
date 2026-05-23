// Package webhook implements the Mutating Admission Webhook for
// topology-aware Pod mutation.
//
// It provides:
//   - HandleMutate: HTTP handler for /mutate endpoint
//   - EvaluatePolicy: Policy engine mapping annotations to scheduling constructs
//   - PatchOperation: RFC 6902 JSON Patch builder
//
// This package will be fully implemented in Step 4.
package webhook
