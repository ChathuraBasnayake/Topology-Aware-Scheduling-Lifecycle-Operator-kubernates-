// Package main starts the custom topology controller.
package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"
	"time"

	"github.com/topology-operator/pkg/controller"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/klog/v2"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

func main() {
	// Init logging
	klog.InitFlags(nil)
	defer klog.Flush()

	// Parse flags for kubeconfig
	var kubeconfig string
	var masterURL string
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to a kubeconfig. Only required if out-of-cluster.")
	flag.StringVar(&masterURL, "master", "", "The address of the Kubernetes API server. Overrides any value in kubeconfig.")
	flag.Parse()

	// 1. Build REST config
	cfg, err := clientcmd.BuildConfigFromFlags(masterURL, kubeconfig)
	if err != nil {
		klog.Fatalf("Error building kubeconfig: %v", err)
	}

	// 2. Create Clientset for standard K8s resources (Nodes, Pods)
	kubeClient, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building kubernetes clientset: %v", err)
	}

	// 3. Create Clientset for Metrics Server telemetry (metrics.k8s.io)
	metricsClient, err := metricsv.NewForConfig(cfg)
	if err != nil {
		klog.Fatalf("Error building metrics clientset: %v", err)
	}

	// 4. Create Informer Factory with 30-second resync interval
	informerFactory := informers.NewSharedInformerFactory(kubeClient, 30*time.Second)

	// 5. Initialize the Topology Controller
	ctrl := controller.NewTopologyController(
		kubeClient,
		metricsClient,
		informerFactory.Core().V1().Nodes(),
		informerFactory.Core().V1().Pods(),
	)

	// 6. Setup Graceful Shutdown context
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// 7. Start Informers
	informerFactory.Start(ctx.Done())

	// 8. Run Controller (using 2 workers for concurrent reconciliation)
	if err := ctrl.Run(ctx, 2); err != nil {
		klog.Fatalf("Error running controller: %v", err)
	}
}
