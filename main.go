package main

import (
	"github.com/OpenPlatformSDN/nuage-cni-k8s/config"
	k8s "github.com/OpenPlatformSDN/nuage-cni-k8s/kubernetes-client"
	vsd "github.com/OpenPlatformSDN/nuage-cni-k8s/vsd-client"

	"flag"
	"fmt"
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
	// kubeconfig     = flag.String("kubeconfig", "./kubeconfig", "absolute path to the kubeconfig file")
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
	glog.Infof("===> log_dir: %#v\n", *flag.CommandLine.Lookup("log_dir"))

	if err := config.LoadAgentConfig(Config); err != nil {
		glog.Fatalf("Cannot read configuration file: %s", err)

	}

	fmt.Printf("===> Agent Configuration: %#v\n", *Config)

	//// blocking etcd client here

	go vsd.Client(Config)
	go k8s.Client(Config)

	select {}

}
