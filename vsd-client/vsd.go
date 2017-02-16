package vsd

import (
	"crypto/tls"
	"encoding/json"

	"github.com/golang/glog"

	"github.com/OpenPlatformSDN/nuage-cni-k8s/config"

	"github.com/nuagenetworks/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

var (
	// Nuage API connection defaults. We need to keep them as global vars since commands can be invoked in whatever order.

	root      *vspk.Me
	mysession *bambou.Session

	// Nuage Enterprise and Domain for this K8S cluster. Non-nil pointers only if valid / found
	enterprise *vspk.Enterprise
	domain     *vspk.Domain
)

////////
////////
////////

func Client(conf *config.AgentConfig) {

	if err := MakeX509conn(conf); err != nil {
		glog.Fatalf("Nuage TLS API connection failed, aborting. Error: %s", err)
	}

	if conf.NuageConfig.Enterprise == "" || conf.NuageConfig.Domain == "" {
		glog.Fatal("Nuage VSD Enterprise and/or Domain for the Kubernetes absent from configuration file, aborting")
	}

	//// Find/Create VSD Enterprise and Domain

	//// VSD Enterprise
	if el, err := root.Enterprises(&bambou.FetchingInfo{Filter: "name == \"" + conf.NuageConfig.Enterprise + "\""}); err != nil {
		glog.Fatalf("Error fetching list of Enterprises from the VSD. Error: %s", err)
	} else {
		switch len(el) {
		case 1: // Given Enterprise already exists
			enterprise = el[0]
			jsonorg, _ := json.MarshalIndent(enterprise, "", "\t")
			glog.Infof("Found Enterprise Name %#s:\n%#s", enterprise.Name, string(jsonorg))
		case 0:
			glog.Infof("VSD Enterprise %#s not found, creating...", conf.NuageConfig.Enterprise)
			enterprise = new(vspk.Enterprise)
			enterprise.Name = conf.NuageConfig.Enterprise
			enterprise.Description = "Automatically created Enterprise for K8S Cluster"
			if err := root.CreateEnterprise(enterprise); err != nil {
				glog.Fatalf("Cannot create VSD Enterprise: %#s. Error: %s", enterprise.Name, err)
			}
			jsonorg, _ := json.MarshalIndent(enterprise, "", "\t")
			glog.Infof("Created Enterprise Name %#s:\n%#s\n", enterprise.Name, string(jsonorg))
		}
	}

	////  VSD Domain
	if dl, err := root.Domains(&bambou.FetchingInfo{Filter: "name == \"" + conf.NuageConfig.Domain + "\""}); err != nil {
		glog.Fatalf("Error fetching list of Domains from the VSD. Error: %s", err)
	} else {
		switch len(dl) {
		case 1: // Given Domain already exists
			domain = dl[0]
			jsond, _ := json.MarshalIndent(domain, "", "\t")
			glog.Infof("Found Domain Name %#s\n%#s", domain.Name, string(jsond))
		case 0:
			glog.Infof("VSD Domain %#s not found, creating...", conf.NuageConfig.Domain)
			// First, we need a Domain template.
			domaintemplate := new(vspk.DomainTemplate)
			domaintemplate.Name = "Template for Domain " + conf.NuageConfig.Domain
			if err := enterprise.CreateDomainTemplate(domaintemplate); err != nil {
				glog.Fatalf("Cannot create VSD Domain Template: %#s. Error: %s", domaintemplate.Name, err)
			}
			// Create Domain under that template
			domain = new(vspk.Domain)
			domain.Name = conf.NuageConfig.Domain
			domain.Description = "Automatically created Domain for K8S Cluster"
			domain.TemplateID = domaintemplate.ID
			// enterprise is valid by now
			if err := enterprise.CreateDomain(domain); err != nil {
				glog.Fatalf("Cannot create VSD Domain: %#s. Error: %s", domain.Name, err)
			}
			jsond, _ := json.MarshalIndent(domain, "", "\t")
			glog.Infof("Created Domain Name %#s\n%#s", domain.Name, string(jsond))
		}
	}

	/////

	select {}

}

////
func MakeX509conn(conf *config.AgentConfig) error {
	if cert, err := tls.LoadX509KeyPair(conf.NuageConfig.CertFile, conf.NuageConfig.KeyFile); err != nil {
		return err
	} else {
		mysession, root = vspk.NewX509Session(&cert, conf.NuageConfig.VsdUrl)
	}

	glog.Infof("===> My VSD URL is: %s\n", conf.NuageConfig.VsdUrl)

	glog.Infof("===> My Bambou session is: %#v\n", *mysession)

	// mysession.SetInsecureSkipVerify(true)

	if err := mysession.Start(); err != nil {
		return err
	}
	return nil
}

////
func resetconn() {
	if mysession != nil {
		mysession.Reset()
	}
	root = nil
	enterprise = nil
	domain = nil
}
