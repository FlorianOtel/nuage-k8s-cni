package config

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"

	yaml "gopkg.in/yaml.v2"
)

// nuage-cni-k8s-master -- configuration file
type AgentConfig struct {
	// Not supplied in YAML config file
	ConfigFile string `yaml:"-"`
	// Config file fields
	KubeConfigFile   string      `yaml:"nuage-k8s-agent-kubeconfig"`
	MasterConfigFile string      `yaml:"k8s-master-config"`
	NuageConfig      nuageConfig `yaml:"nuage-config"`
}

type nuageConfig struct {
	VsdUrl     string `yaml:"vsd-url"`
	APIVersion string `yaml:"apiversion"`
	Enterprise string `yaml:"enterprise"`
	Domain     string `yaml:"domain"`
	CertFile   string `yaml:"certFile"`
	KeyFile    string `yaml:"keyFile"`
}

////////
//////// Parts from the K8S master config file we are interested in
////////

type masterConfig struct {
	NetworkConfig  networkConfig  `yaml:"networkConfig"`
	EtcdClientInfo etcdClientInfo `yaml:"etcdClientInfo"`
}

type networkConfig struct {
	ClusterCIDR  string `yaml:"clusterNetworkCIDR"`
	SubnetLength int    `yaml:"hostSubnetLength"`
	ServiceCIDR  string `yaml:"serviceNetworkCIDR"`
}

// follow K8S master denomination instead of naming consistency
type etcdClientInfo struct {
	EtcdCA         string   `yaml:"ca"`
	EtcdCertFile   string   `yaml:"certFile"`
	EtcdKeyFile    string   `yaml:"keyFile"`
	EtcdServerUrls []string `yaml:"urls"`
}

////////
////////
////////

func Flags(conf *AgentConfig, flagSet *flag.FlagSet) {
	// Reminder
	// agentname := "nuage-k8s-agent"
	//
	flagSet.StringVar(&conf.ConfigFile, "config",
		"./nuage-k8s-agent-config.yaml", "configuration file for Nuage Kubernetes agent. If this file is specified, all remaining arguments will be ignored")
	flagSet.StringVar(&conf.KubeConfigFile, "kubeconfig",
		"./nuage-k8s-agent.kubeconfig", "kubeconfig file for Nuage Kuberenetes agent")
	flagSet.StringVar(&conf.MasterConfigFile, "masterconfig",
		"", "Kubernetes master node configuration file")

	flagSet.StringVar(&conf.NuageConfig.VsdUrl, "nuageurl",
		"", "Nuage VSD URL")

	flagSet.StringVar(&conf.NuageConfig.APIVersion, "nuageapi",
		"v4_0", "Nuage VSP API Version")
	flagSet.StringVar(&conf.NuageConfig.Enterprise, "nuageenterprise",
		"", "Nuage Enterprise Name for the Kuberenetes cluster")
	flagSet.StringVar(&conf.NuageConfig.Domain, "nuagedomain",
		"", "Nuage Domain Name for the Kuberenetes cluster")
	flagSet.StringVar(&conf.NuageConfig.CertFile, "nuagecertfile",
		"./nuage-k8s-agent.crt", "Nuage Kubernetes agent VSD certificate file")
	flagSet.StringVar(&conf.NuageConfig.KeyFile, "nuagekeyfile",
		"./nuage-k8s-agent.key", "Nuage Kubernetes agent VSD private key file")
	// Set the values for log_dir and logtostderr.  Because this happens before flag.Parse(), cli arguments will override these.
	// Also set the DefValue parameter so -help shows the new defaults.
	// XXX - Make sure "glog" package is imported at this point, otherwise this will panic
	log_dir := flagSet.Lookup("log_dir")
	log_dir.Value.Set(fmt.Sprintf("/var/log/%s", path.Base(os.Args[0])))
	log_dir.DefValue = fmt.Sprintf("/var/log/%s", path.Base(os.Args[0]))
	logtostderr := flagSet.Lookup("logtostderr")
	logtostderr.Value.Set("false")
	logtostderr.DefValue = "false"
	stderrlogthreshold := flagSet.Lookup("stderrthreshold")
	stderrlogthreshold.Value.Set("3")
	stderrlogthreshold.DefValue = "3"
	log.SetFlags(log.LstdFlags | log.Lshortfile)
}

func LoadAgentConfig(conf *AgentConfig) error {
	data, err := ioutil.ReadFile(conf.ConfigFile)
	if err != nil {
		return err
	}

	if err := yaml.Unmarshal(data, conf); err != nil {
		return err
	}

	return nil
}
