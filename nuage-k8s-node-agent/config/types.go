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

// nuage-k8s-node-agent-config.yaml -- configuration file
type AgentConfig struct {
	ConfigFile string `yaml:"-"`
	// Config file fields
	VrsConfig   string      `yaml:"vrs-config"`
	NuageConfig nuageConfig `yaml:"nuage-config"`
}

type nuageConfig struct {
	ServerUrl string `yaml:"server-url"`
	CaFile    string `yaml:"ca"`
	CertFile  string `yaml:"certFile"`
	KeyFile   string `yaml:"keyFile"`
}

////////
////////
////////

func Flags(conf *AgentConfig, flagSet *flag.FlagSet) {
	// Reminder
	// agentname := "nuage-k8s-node-agent"
	//
	flagSet.StringVar(&conf.ConfigFile, "config",
		"./nuage-k8s-node-agent-config.yaml", "configuration file for Nuage Kubernetes nodes agent. If this file is specified, all remaining arguments will be ignored")
	flagSet.StringVar(&conf.VrsConfig, "vrsconfig",
		"", "Nuage VRS configuration....")

	flagSet.StringVar(&conf.NuageConfig.ServerUrl, "serverurl",
		"https://0.0.0.0:7443", "Nuage Kubernetes node agent URL")
	flagSet.StringVar(&conf.NuageConfig.CaFile, "cafile",
		"./nuage-k8s-node-agent.ca", "Nuage Kubernetes agent server CA certificate file")
	flagSet.StringVar(&conf.NuageConfig.CertFile, "certfile",
		"./nuage-k8s-node-agent.crt", "Nuage Kubernetes agent server certificate file")
	flagSet.StringVar(&conf.NuageConfig.KeyFile, "keyfile",
		"./nuage-k8s-node-agent.key", "Nuage Kubernetes agent server private key file")
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
