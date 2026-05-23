// Package controller implements the custom controller that watches Nodes and Pods,
// queries live telemetry from the Metrics Server, and writes real-time topology
// metrics as Node annotations.
package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/klog/v2"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"
)

// TopologyController watches Nodes and Pods, queries live telemetry from
// the Metrics Server, and writes real-time topology metrics as Node annotations.
type TopologyController struct {
	kubeClient    kubernetes.Interface
	metricsClient metricsv.Interface // Live telemetry: queries metrics.k8s.io

	// Listers provide read-only access to the local cache.
	nodeLister  listersv1.NodeLister
	nodesSynced cache.InformerSynced // Returns true when cache is populated

	podLister  listersv1.PodLister
	podsSynced cache.InformerSynced

	// Rate-limiting workqueue handles deduplication and exponential backoff
	workqueue workqueue.RateLimitingInterface

	// Live topology aggregator calculates metrics from real-time telemetry
	aggregator *LiveTopologyAggregator
}

// NewTopologyController creates a new controller and registers event handlers.
func NewTopologyController(
	kubeClient kubernetes.Interface,
	metricsClient metricsv.Interface,
	nodeInformer coreinformers.NodeInformer,
	podInformer coreinformers.PodInformer,
) *TopologyController {
	ctrl := &TopologyController{
		kubeClient:    kubeClient,
		metricsClient: metricsClient,
		nodeLister:    nodeInformer.Lister(),
		nodesSynced:   nodeInformer.Informer().HasSynced,
		podLister:     podInformer.Lister(),
		podsSynced:    podInformer.Informer().HasSynced,
		workqueue:     workqueue.NewNamedRateLimitingQueue(
			workqueue.DefaultControllerRateLimiter(),
			"topology-controller",
		),
		aggregator:    NewLiveTopologyAggregator(),
	}

	// ---- Register Node Event Handlers ----
	nodeInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(obj)
			if err != nil {
				utilruntime.HandleError(err)
				return
			}
			ctrl.workqueue.Add(key)
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			key, err := cache.MetaNamespaceKeyFunc(newObj)
			if err != nil {
				utilruntime.HandleError(err)
				return
			}
			ctrl.workqueue.Add(key)
		},
	})

	// ---- Register Pod Event Handlers ----
	podInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj interface{}) {
			pod := obj.(*corev1.Pod)
			if pod.Spec.NodeName != "" {
				ctrl.workqueue.Add(pod.Spec.NodeName)
			}
		},
		UpdateFunc: func(oldObj, newObj interface{}) {
			pod := newObj.(*corev1.Pod)
			if pod.Spec.NodeName != "" {
				ctrl.workqueue.Add(pod.Spec.NodeName)
			}
		},
		DeleteFunc: func(obj interface{}) {
			pod, ok := obj.(*corev1.Pod)
			if !ok {
				tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
				if !ok {
					return
				}
				pod, ok = tombstone.Obj.(*corev1.Pod)
				if !ok {
					return
				}
			}
			if pod.Spec.NodeName != "" {
				ctrl.workqueue.Add(pod.Spec.NodeName)
			}
		},
	})

	return ctrl
}

// Run starts the controller's worker goroutines.
func (c *TopologyController) Run(ctx context.Context, workers int) error {
	defer utilruntime.HandleCrash()
	defer c.workqueue.ShutDown()

	klog.Info("Starting Topology Controller")

	klog.Info("Waiting for informer caches to sync...")
	if ok := cache.WaitForCacheSync(ctx.Done(), c.nodesSynced, c.podsSynced); !ok {
		return fmt.Errorf("failed to wait for caches to sync")
	}
	klog.Info("Informer caches synced successfully")

	for i := 0; i < workers; i++ {
		go wait.UntilWithContext(ctx, c.runWorker, time.Second)
	}

	klog.Info("Controller started, waiting for events...")
	<-ctx.Done()
	klog.Info("Shutting down controller")
	return nil
}

func (c *TopologyController) runWorker(ctx context.Context) {
	for c.processNextItem(ctx) {
	}
}

// processNextItem pulls one key from the queue and reconciles it.
func (c *TopologyController) processNextItem(ctx context.Context) bool {
	key, quit := c.workqueue.Get()
	if quit {
		return false
	}
	defer c.workqueue.Done(key)

	err := c.syncHandler(ctx, key.(string))
	if err == nil {
		c.workqueue.Forget(key)
		return true
	}

	if c.workqueue.NumRequeues(key) < 5 {
		klog.Warningf("Error syncing node %s, retrying: %v", key, err)
		c.workqueue.AddRateLimited(key)
	} else {
		klog.Errorf("Dropping node %s after 5 retries: %v", key, err)
		c.workqueue.Forget(key)
	}

	return true
}

// syncHandler reconciles a single node's topology annotations based on live telemetry.
func (c *TopologyController) syncHandler(ctx context.Context, nodeName string) error {
	node, err := c.nodeLister.Get(nodeName)
	if err != nil {
		if errors.IsNotFound(err) {
			klog.Infof("Node %s was deleted, skipping", nodeName)
			return nil
		}
		return fmt.Errorf("failed to get node %s: %v", nodeName, err)
	}

	// Query live telemetry from the Metrics Server
	nodeMetrics, err := c.metricsClient.MetricsV1beta1().NodeMetricses().Get(
		ctx, nodeName, metav1.GetOptions{},
	)
	if err != nil {
		klog.Warningf("Live metrics not available for node %s: %v", nodeName, err)
		return err
	}

	annotations := c.aggregator.BuildLiveAnnotations(node, nodeMetrics)

	if !annotationsChanged(node.Annotations, annotations) {
		return nil
	}

	patchData := fmt.Sprintf(`{"metadata":{"annotations":%s}}`, mustMarshal(annotations))

	_, err = c.kubeClient.CoreV1().Nodes().Patch(
		ctx,
		nodeName,
		types.StrategicMergePatchType,
		[]byte(patchData),
		metav1.PatchOptions{},
	)
	if err != nil {
		return fmt.Errorf("failed to patch node %s: %v", nodeName, err)
	}

	klog.Infof("Updated LIVE topology annotations for node %s: cpu=%s%%, mem=%s%%, health=%s",
		nodeName,
		annotations["topology-aware.io/cpu-utilization"],
		annotations["topology-aware.io/memory-utilization"],
		annotations["topology-aware.io/health-score"])

	return nil
}

func annotationsChanged(oldAnn, newAnn map[string]string) bool {
	if oldAnn == nil {
		return true
	}
	for k, v := range newAnn {
		if oldAnn[k] != v {
			return true
		}
	}
	return false
}

func mustMarshal(obj interface{}) string {
	bytes, err := json.Marshal(obj)
	if err != nil {
		panic(err)
	}
	return string(bytes)
}
