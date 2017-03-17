package main

import (
	"flag"
	"os"
	"path"

	cniclient "github.com/OpenPlatformSDN/nuage-k8s-cni/cni-agent-client"
	etcdclient "github.com/OpenPlatformSDN/nuage-k8s-cni/etcd-client"

	"github.com/OpenPlatformSDN/nuage-k8s-cni/config"
	k8sclient "github.com/OpenPlatformSDN/nuage-k8s-cni/k8s-client"
	vsdclient "github.com/OpenPlatformSDN/nuage-k8s-cni/vsd-client"

	"github.com/golang/glog"
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

	if err := etcdclient.InitClient(Config); err != nil {
		glog.Errorf("ETCD client error: %s", err)
		os.Exit(255)
	}

	// XXX  -- This will block until we get a Leader lock from etcd

	etcdclient.LeaderElection()

	if err := vsdclient.InitClient(Config); err != nil {
		glog.Errorf("VSD client error: %s", err)
		os.Exit(255)
	}

	if err := cniclient.InitClient(Config); err != nil {
		glog.Errorf("CNI angent client error: %s", err)
		os.Exit(255)
	}

	if err := k8sclient.InitClient(Config); err != nil {
		glog.Errorf("Kubernetes client error: %s", err)
		os.Exit(255)
	}

	go k8sclient.EventWatcher()

	select {}

}
