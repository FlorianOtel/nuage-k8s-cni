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

// nuage-k8s-cni-master -- configuration file
type AgentConfig struct {

	// Not supplied in YAML config file
	EtcdServerUrl string `yaml:"-"` // May also be specified by EtcdClientInfo below. Overriden by the latter if valid.
	ConfigFile    string `yaml:"-"`
	// Config file fields
	KubeConfigFile   string    `yaml:"nuage-k8s-master-agent-kubeconfig"`
	MasterConfigFile string    `yaml:"k8s-master-config"`
	VsdConfig        vsdConfig `yaml:"vsd-config"`
	CniConfig        cniConfig `yaml:"cni-config"`
}

type vsdConfig struct {
	VsdUrl     string `yaml:"vsd-url"`
	APIVersion string `yaml:"apiversion"`
	Enterprise string `yaml:"enterprise"`
	Domain     string `yaml:"domain"`
	CertFile   string `yaml:"certFile"`
	KeyFile    string `yaml:"keyFile"`
}

type cniConfig struct {
	ServerPort string `yaml:"server-port"` // CNI Agent server port
	CaFile     string `yaml:"caFile"`      // CNI Agent server CA certificate
}

////////
//////// Parts from the K8S master config file we are interested in
////////

type MasterConfig struct {
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
	// agentname := "nuage-k8s-master-agent"
	//
	flagSet.StringVar(&conf.ConfigFile, "config",
		"./nuage-k8s-master-agent-config.yaml", "configuration file for Nuage Kubernetes masters agent. If this file is specified, all remaining arguments will be ignored")
	flagSet.StringVar(&conf.EtcdServerUrl, "etcd-server",
		"", "etcd Server URL. If Kubernetes Master configuration file contains etcd client info, that information will be used instead")
	flagSet.StringVar(&conf.KubeConfigFile, "kubeconfig",
		"./nuage-k8s-master-agent.kubeconfig", "kubeconfig file for Nuage Kuberenetes masters agent")
	flagSet.StringVar(&conf.MasterConfigFile, "masterconfig",
		"", "Kubernetes masters configuration file")
	// CNI flags
	flagSet.StringVar(&conf.CniConfig.ServerPort, "cniserverport",
		"7443", "server port for Kubernetes nodes Nuage CNI Agent server")
	flagSet.StringVar(&conf.CniConfig.CaFile, "cniserverca",
		"/opt/nuage/etc/ca.crt", "CA file for Kubernetes nodes Nuage CNI Agent server")
	// VSD flags
	flagSet.StringVar(&conf.VsdConfig.VsdUrl, "vsdurl",
		"", "Nuage VSD URL")
	flagSet.StringVar(&conf.VsdConfig.APIVersion, "vsdapiversion",
		"v4_0", "Nuage VSP API Version")
	flagSet.StringVar(&conf.VsdConfig.Enterprise, "vsdenterprise",
		"", "Nuage Enterprise Name for the Kuberenetes cluster")
	flagSet.StringVar(&conf.VsdConfig.Domain, "vsddomain",
		"", "Nuage Domain Name for the Kuberenetes cluster")
	flagSet.StringVar(&conf.VsdConfig.CertFile, "vsdcertfile",
		"./nuage-k8s-master-agent.crt", "VSD certificate file for Nuage Kubernetes masters agent")
	flagSet.StringVar(&conf.VsdConfig.KeyFile, "vsdkeyfile",
		"./nuage-k8s-master-agent.key", "VSD private key file for Nuage Kubernetes masters agent")
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
	stderrlogthreshold.Value.Set("2")
	stderrlogthreshold.DefValue = "2"
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
