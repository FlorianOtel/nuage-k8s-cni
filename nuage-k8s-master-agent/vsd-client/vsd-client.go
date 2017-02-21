package vsd

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"io/ioutil"
	"net"

	yaml "gopkg.in/yaml.v2"

	"github.com/golang/glog"

	"github.com/OpenPlatformSDN/nuage-cni-k8s/nuage-k8s-master-agent/config"

	"github.com/nuagenetworks/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

const (
	MAX_SUBNETS = 2048 // Practical, safety max limit on nr Subnets we handle (upper limit for 1<< SubnetLength)
	////
	//// Patterns for K8S construct naming in VSD
	////
	ZONE_NAME = "K8S namespace "
	NMG_NAME  = "K8S services in namespace " // Network Macro Group Name
	NM_NAME   = "K8S service "               // Network Macro Name
	POD_NAME  = "K8S pod "
)

var (
	// Nuage API connection defaults. We need to keep them as global vars since commands can be invoked in whatever order.

	root      *vspk.Me
	mysession *bambou.Session

	// K8S Master config -- includes network information and etcd client details
	masterconfig config.MasterConfig

	// K8S cluster network configuration details
	// ClusterCIDR  *net.IPNet
	// SubnetLength int

	// Nuage Enterprise and Domain for this K8S cluster. Created if they don't exist already
	Enterprise *vspk.Enterprise
	Domain     *vspk.Domain

	//// XXX - VSD view of things. Must be reconciled with K8S data

	Zones      map[string]*vspk.Zone              // Key: ZONE_NAME + Name
	Subnets    map[string]*vspk.Subnet            // Key: (!!) vspk.Subnet.Address (subnet prefix as string)
	Containers map[string]*vspk.Container         // Key: POD_NAME + Name
	NMGs       map[string]*vspk.NetworkMacroGroup // Key: NMG_NAME + Name
	NMs        map[string]*vspk.EnterpriseNetwork // Key: NM_NAME + Name

	// (Sub)set of allowed prefixes for Pod subnets
	prefixes map[string]*vspk.Subnet
)

func InitClient(conf *config.AgentConfig) error {
	if err := readnetconfig(conf); err != nil {
		return err
	}

	if err := makeX509conn(conf); err != nil {
		return bambou.NewBambouError("Nuage TLS API connection failed", err.Error())
	}

	if conf.NuageConfig.Enterprise == "" || conf.NuageConfig.Domain == "" {
		return bambou.NewBambouError("Nuage VSD Enterprise and/or Domain for the Kubernetes cluster is absent from configuration file", "")
	}

	//// Find/Create VSD Enterprise and Domain

	//// VSD Enterprise
	if el, err := root.Enterprises(&bambou.FetchingInfo{Filter: "name == \"" + conf.NuageConfig.Enterprise + "\""}); err != nil {
		return bambou.NewBambouError("Error fetching list of Enterprises from the VSD", err.Error())
	} else {
		switch len(el) {
		case 1: // Given Enterprise already exists
			Enterprise = el[0]
			jsonorg, _ := json.MarshalIndent(Enterprise, "", "\t")
			glog.Infof("Found Enterprise Name %#s:\n%#s", Enterprise.Name, string(jsonorg))
		case 0:
			glog.Infof("VSD Enterprise %#s not found, creating...", conf.NuageConfig.Enterprise)
			Enterprise = new(vspk.Enterprise)
			Enterprise.Name = conf.NuageConfig.Enterprise
			Enterprise.Description = "Automatically created Enterprise for K8S Cluster"
			if err := root.CreateEnterprise(Enterprise); err != nil {
				return bambou.NewBambouError("Cannot create VSD Enterprise: "+Enterprise.Name, err.Error())
			}
			jsonorg, _ := json.MarshalIndent(Enterprise, "", "\t")
			glog.Infof("Created Enterprise Name %#s:\n%#s\n", Enterprise.Name, string(jsonorg))
		}
	}

	////  VSD Domain
	if dl, err := root.Domains(&bambou.FetchingInfo{Filter: "name == \"" + conf.NuageConfig.Domain + "\""}); err != nil {
		return bambou.NewBambouError("Error fetching list of Domains from the VSD", err.Error())
	} else {
		switch len(dl) {
		case 1: // Given Domain already exists
			Domain = dl[0]
			jsond, _ := json.MarshalIndent(Domain, "", "\t")
			glog.Infof("Found Domain Name %#s\n%#s", Domain.Name, string(jsond))
		case 0:
			glog.Infof("VSD Domain %#s not found, creating...", conf.NuageConfig.Domain)
			// First, we need a Domain template.
			domaintemplate := new(vspk.DomainTemplate)
			domaintemplate.Name = "Template for Domain " + conf.NuageConfig.Domain
			if err := Enterprise.CreateDomainTemplate(domaintemplate); err != nil {
				return bambou.NewBambouError("Cannot create VSD Domain Template: "+domaintemplate.Name, err.Error())
			}
			// Create Domain under this template
			Domain = new(vspk.Domain)
			Domain.Name = conf.NuageConfig.Domain
			Domain.Description = "Automatically created Domain for K8S Cluster"
			Domain.TemplateID = domaintemplate.ID
			// Enterprise is valid by now
			if err := Enterprise.CreateDomain(Domain); err != nil {
				return bambou.NewBambouError("Cannot create VSD Domain: "+Domain.Name, err.Error())
			}
			jsond, _ := json.MarshalIndent(Domain, "", "\t")
			glog.Infof("Created Domain Name %#s\n%#s", Domain.Name, string(jsond))
		}
	}

	////////
	//////// Parse: Existing VSD Zones
	////////

	Zones = make(map[string]*vspk.Zone)
	if zl, err := Domain.Zones(&bambou.FetchingInfo{}); err != nil {
		return bambou.NewBambouError("Error fetching list of Zones for Domain: "+Domain.Name+" from the VSD", err.Error())
	} else {

		for _, zone := range zl {
			glog.Infof("Found existing Zone with Name: %#s. Caching...", zone.Name)
			Zones[zone.Name] = zone
		}
	}

	////////
	//////// Parse: Existing VSD Subnets
	////////

	// XXX - Create the "prefixes" map with a predefined (MAX_SUBNETS) nr of per-namespace CIDRs based on the values in the K8S master configuration file
	// The actual VSD Subnets with those prefixes are created on-demand (map value non-nil)
	if err := initprefixes(conf); err != nil {
		return err
	}

	if sl, err := Domain.Subnets(&bambou.FetchingInfo{}); err != nil {
		return bambou.NewBambouError("Error fetching list of Subnets for Domain: "+Domain.Name+" from the VSD", err.Error())
	} else {
		for _, subnet := range sl {
			glog.Infof("Found existing Subnet with Name: %#s. Caching...", subnet.Name)
			// Mark this Subnet prefix as allocated already
			/// .... Check the nr of interfaces in this subnet, if any.
			prefixes[subnet.Address] = subnet
		}
	}

	////////
	//////// Parse: Existing Network Macro Groups
	////////
	NMGs = make(map[string]*vspk.NetworkMacroGroup)

	if nmgs, err := Enterprise.NetworkMacroGroups(&bambou.FetchingInfo{}); err != nil {
		return bambou.NewBambouError("Error fetching list of Network Macro Groups for Enterprise: "+Enterprise.Name+" from the VSD", err.Error())
	} else {
		for _, nmg := range nmgs {
			glog.Infof("Found existing Network Macro Group with Name: %#s. Caching...", nmg.Name)
			NMGs[nmg.Name] = nmg
		}
	}

	////////
	//////// Parse: Existing Network Macros
	////////
	NMs = make(map[string]*vspk.EnterpriseNetwork)

	if nms, err := Enterprise.EnterpriseNetworks(&bambou.FetchingInfo{}); err != nil {
		return bambou.NewBambouError("Error fetching list of Network Macros for Enterprise: "+Enterprise.Name+" from the VSD", err.Error())
	} else {
		for _, nm := range nms {
			glog.Infof("Found existing Network Macro with Name: %#s. Caching...", nm.Name)
			NMs[nm.Name] = nm
		}
	}

	////////
	//////// Parse: Existing VSD Containers
	////////

	Containers = make(map[string]*vspk.Container)
	if cl, err := Domain.Containers(&bambou.FetchingInfo{}); err != nil {
		return bambou.NewBambouError("Error fetching list of Containers for Domain: "+Domain.Name+" from the VSD", err.Error())
	} else {

		for _, container := range cl {
			glog.Infof("Found existing Container with Name: %#s. Caching...", container.Name)
			Containers[container.Name] = container
		}
	}

	/*
		range1 := ipallocator.NewCIDRRange(clustercidr)
		glog.Infof("====> Range1: %#v : %s,", *range1, range1.CIDR())
		range2 := ipallocator.NewCIDRRange(clustercidr)
		glog.Infof("====> Range2: %#v : %s,", *range2, range2.CIDR())
		range3 := ipallocator.NewCIDRRange(clustercidr)
		glog.Infof("====> Range3: %#v : %s,", *range3, range3.CIDR())
		range4 := ipallocator.NewCIDRRange(clustercidr)
		glog.Infof("====> Range4: %#v : %s,", *range4, range4.CIDR())
	*/

	glog.Info("VSD client initialization completed")
	return nil
}

/*

	///// Find / initialize the Zones for "priviledged" and "default" K8S namespaces

	// Find/Create k8s.PrivilegedNS
	if zl, err := Domain.Zones(&bambou.FetchingInfo{Filter: "name == \"" + k8s.PrivilegedNS + "\""}); err != nil {
		return bambou.NewBambouError("Error fetching list of Zones from the VSD", err.Error())
	}

	switch len(zl) {
	case 1:
		// Zone already exists
		glog.Infof("Found existing Zone for K8S Namespace: %#s", k8s.PrivilegedNS)
		K8Sns[k8s.PrivilegedNS] = zl[0]
	}


*/

////////
//////// utils
////////

////  Load K8S Master configuration file -- NetworkingConfig and EtcdClientInfo
func readnetconfig(conf *config.AgentConfig) error {
	if data, err := ioutil.ReadFile(conf.MasterConfigFile); err != nil {
		return bambou.NewBambouError("Cannot read K8S Master configuration file: "+conf.MasterConfigFile, err.Error())
	} else {
		if err := yaml.Unmarshal(data, &masterconfig); err != nil {
			return bambou.NewBambouError("Cannot parse K8S Master configuration file: "+conf.MasterConfigFile, err.Error())
		}
	}
	return nil
}

// Create a connection to the VSD using X.509 certificate-based authentication
func makeX509conn(conf *config.AgentConfig) error {
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

//// Initialize the "prefixes" map with up to MAX_SUBNETS number of prefixes, based on the values of "ClusterCIDR" and "SubnetLength" (sanity checked)
func initprefixes(conf *config.AgentConfig) error {
	var err error
	var ccidr *net.IPNet

	if _, ccidr, err = net.ParseCIDR(masterconfig.NetworkConfig.ClusterCIDR); err != nil {
		return bambou.NewBambouError("Cannot parse K8S cluster network configuration: "+masterconfig.NetworkConfig.ClusterCIDR, err.Error())

	}
	glog.Infof("====> K8S master config details: %#v", masterconfig)
	glog.Infof("====> Cluster CIDR prefix : %#v , %s", ccidr, ccidr)
	cmask, _ := ccidr.Mask.Size() // Nr bits in the ClusterCIDR prefix mask

	// The resulting subnet mask length for the Pod Subnets in the cluster
	smask := uint(cmask + masterconfig.NetworkConfig.SubnetLength)

	if smask >= 32 {
		glog.Errorf("Invalid resulting subnet mask length for Pod networks: /%d", smask)
	}

	// else {
	// 	nscidr = &net.IPNet{clustercidr.IP, net.CIDRMask(nsmask, 32)}
	// 	glog.Infof("==> Resulting per namespace CIDR: %s", nscidr)
	// }

	//////// Intialize "prefixes" map. Values:
	//////// - Nr Subnets: 1<<SubnetLength  (limited to MAX_SUBNETS)
	//////// - Nr hosts per subnet: 1<<(32-smask)  (incl net addr + broadcast)
	////////
	//////// Easiest way to generate the subnet prefixes is to convert them to/from int32 in "nr hosts per subnet" increments

	prefixes = make(map[string]*vspk.Subnet)
	for i := 0; i < 1<<uint(masterconfig.NetworkConfig.SubnetLength) && i < MAX_SUBNETS; i++ {
		newprefix := intToIP(ipToInt(ccidr.IP) + int32(i*(1<<(32-smask))))
		glog.Infof("=> Generated Subnet Prefix:  %s", newprefix)
		prefixes[string(newprefix)] = nil
	}

	return nil
}

// Converts a 4 bytes IP into a 32 bit integer
func ipToInt(ip net.IP) int32 {
	return int32(binary.BigEndian.Uint32(ip.To4()))
}

// Converts 32 bit integer into a 4 bytes IP address
func intToIP(n int32) net.IP {
	b := make([]byte, 4)
	binary.BigEndian.PutUint32(b, uint32(n))
	return net.IP(b)
}

////
func resetconn() {
	if mysession != nil {
		mysession.Reset()
	}
	root = nil
	Enterprise = nil
	Domain = nil
}
