// Package main starts the custom kube-scheduler binary with the
// TopologyAwarePlacement plugin registered.
package main

import (
	"os"

	"github.com/topology-operator/pkg/scheduler"
	"k8s.io/component-base/cli"
	"k8s.io/kubernetes/cmd/kube-scheduler/app"
)

func main() {
	// Create the kube-scheduler command registering our custom plugin
	command := app.NewSchedulerCommand(
		app.WithPlugin(scheduler.Name, scheduler.New),
	)

	// Run the scheduler CLI command
	code := cli.Run(command)
	os.Exit(code)
}
