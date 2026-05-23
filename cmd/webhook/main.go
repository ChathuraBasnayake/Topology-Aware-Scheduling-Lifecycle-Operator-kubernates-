// Package main starts the HTTPS admission webhook server.
package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os/signal"
	"syscall"

	"github.com/topology-operator/pkg/webhook"
	"k8s.io/klog/v2"
)

func main() {
	// ---------- 1. Load TLS Certificates ----------
	// These are mounted from a Kubernetes Secret into the container at:
	//   /etc/webhook/certs/tls.crt  (server certificate)
	//   /etc/webhook/certs/tls.key  (private key)
	//
	// The certificate's SAN (Subject Alternative Name) MUST match the
	// Kubernetes Service DNS: topology-webhook.topology-system.svc
	// Otherwise the API server will reject the TLS handshake.
	certFile := "/etc/webhook/certs/tls.crt"
	keyFile := "/etc/webhook/certs/tls.key"

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		klog.Fatalf("Failed to load TLS cert: %v", err)
	}

	// ---------- 2. Register HTTP Handlers ----------
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", webhook.HandleMutate) // Main mutation endpoint
	mux.HandleFunc("/healthz", handleHealthz)        // Liveness probe

	// ---------- 3. Configure HTTPS Server ----------
	server := &http.Server{
		Addr:    ":8443",
		Handler: mux,
		TLSConfig: &tls.Config{
			Certificates: []tls.Certificate{cert},
		},
	}

	// ---------- 4. Graceful Shutdown ----------
	// Kubernetes sends SIGTERM before killing the Pod.
	// We catch it to drain in-flight requests cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	go func() {
		klog.Info("Starting webhook server on :8443")
		if err := server.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
			klog.Fatalf("Server error: %v", err)
		}
	}()

	<-ctx.Done()
	klog.Info("Shutting down webhook server...")
	server.Shutdown(context.Background())
}

func handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprint(w, "ok")
}
