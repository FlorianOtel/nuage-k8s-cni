package main

import (
	"github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/config"
	k8s "github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/k8s-client"
	vsd "github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/vsd-client"

	"flag"
	"os"
	"path"

	"github.com/golang/glog"
	//
	// apiv1 "k8s.io/kubernetes/pkg/api/v1"
	// "k8s.io/kubernetes/pkg/apis/extensions"
	// k8sfields "k8s.io/kubernetes/pkg/fields"
	// k8slabels "k8s.io/kubernetes/pkg/labels"
)

const errorLogLevel = 2

var (
	// Top level Agent Configuration
	Config *config.AgentConfig
	// MasterConfig  = masterConfig{}
	// NetworkConfig = networkConfig{}
	UseNetPolicies = false
)

func main() {

	Config = new(config.AgentConfig)

	config.Flags(Config, flag.CommandLine)
	flag.Parse()

	if len(os.Args) == 1 { // With no arguments, print default usage
		flag.PrintDefaults()
		os.Exit(0)
	}
	// Flush the logs upon exit
	defer glog.Flush()

	glog.Infof("===> Starting %s...", path.Base(os.Args[0]))

	if err := config.LoadAgentConfig(Config); err != nil {
		glog.Errorf("Cannot read configuration file: %s", err)
		os.Exit(255)
	}

	//// blocking etcd client here

	if err := vsd.InitClient(Config); err != nil {
		glog.Errorf("VSD client error: %s", err)
		os.Exit(255)
	}
	if err := k8s.InitClient(Config); err != nil {
		glog.Errorf("Kubernetes client error: %s", err)
		os.Exit(255)
	}

	go k8s.EventWatcher()

	select {}

}
