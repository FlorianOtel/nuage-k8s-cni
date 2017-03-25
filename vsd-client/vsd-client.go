package vsd

import (
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v2"

	"github.com/golang/glog"

	"github.com/OpenPlatformSDN/nuage-k8s-cni/config"

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
)

var (
	// Nuage API connection defaults. We need to keep them as global vars since commands can be invoked in whatever order.

	root      *vspk.Me
	mysession *bambou.Session

	// K8S Master config -- includes network information and etcd client details
	k8sMasterConfig config.MasterConfig

	// Nuage Enterprise and Domain for this K8S cluster. Created if they don't exist already
	Enterprise *vspk.Enterprise
	Domain     *vspk.Domain

	//// XXX - VSD view of things. Must be reconciled with K8S data
	Zones map[string]*Zone              // Key: ZONE_NAME + Name
	NMGs  map[string]*NetworkMacroGroup // Key: NMG_NAME + Name
	NMs   map[string]*NetworkMacro      // Key: NM_NAME + Name

	vsdmutex sync.Mutex // Serialize VSD operations, esp creates/updates

	// (Sub)set of allowed prefixes for Pod subnets w
	FreeCIDRs map[string]*net.IPNet // Key: (!!) vspk.Subnet.Address (subnet prefix as string)
)

func InitClient(conf *config.AgentConfig) error {
	if err := readK8Sconfig(conf); err != nil {
		return err
	}

	if err := makeX509conn(conf); err != nil {
		return bambou.NewBambouError("Nuage TLS API connection failed", err.Error())
	}

	if conf.VsdConfig.Enterprise == "" || conf.VsdConfig.Domain == "" {
		return bambou.NewBambouError("Nuage VSD Enterprise and/or Domain for the Kubernetes cluster is absent from configuration file", "")
	}

	//// Find/Create VSD Enterprise and Domain

	//// VSD Enterprise
	if el, err := root.Enterprises(&bambou.FetchingInfo{Filter: "name == \"" + conf.VsdConfig.Enterprise + "\""}); err != nil {
		return bambou.NewBambouError("Error fetching list of Enterprises from the VSD", err.Error())
	} else {
		switch len(el) {
		case 1: // Given Enterprise already exists
			Enterprise = el[0]
			glog.Infof("Found existing Enterprise: %#s , re-using...", Enterprise.Name)
		case 0:
			glog.Infof("VSD Enterprise %#s not found, creating...", conf.VsdConfig.Enterprise)
			Enterprise = new(vspk.Enterprise)
			Enterprise.Name = conf.VsdConfig.Enterprise
			Enterprise.Description = "Automatically created Enterprise for K8S Cluster"
			if err := root.CreateEnterprise(Enterprise); err != nil {
				return bambou.NewBambouError("Cannot create Enterprise: "+Enterprise.Name, err.Error())
			}
			glog.Infof("Created Enterprise: %s", Enterprise.Name)
		}
	}

	////  VSD Domain
	if dl, err := root.Domains(&bambou.FetchingInfo{Filter: "name == \"" + conf.VsdConfig.Domain + "\""}); err != nil {
		return bambou.NewBambouError("Error fetching list of Domains from the VSD", err.Error())
	} else {
		switch len(dl) {
		case 1: // Given Domain already exists
			Domain = dl[0]
			glog.Infof("Found existing Domain: %#s , re-using...", Domain.Name)
		case 0: // Domain does not exist, create it
			glog.Infof("VSD Domain %#s not found, creating...", conf.VsdConfig.Domain)
			// First, we need a Domain template.
			domaintemplate := new(vspk.DomainTemplate)
			domaintemplate.Name = "Template for Domain " + conf.VsdConfig.Domain
			if err := Enterprise.CreateDomainTemplate(domaintemplate); err != nil {
				return bambou.NewBambouError("Cannot create Domain Template: "+domaintemplate.Name, err.Error())
			}
			// Create Domain under this template
			Domain = new(vspk.Domain)
			Domain.Name = conf.VsdConfig.Domain
			Domain.Description = "Automatically created Domain for K8S Cluster"
			Domain.TemplateID = domaintemplate.ID
			// Enterprise is valid by now
			if err := Enterprise.CreateDomain(Domain); err != nil {
				return bambou.NewBambouError("Cannot create Domain: "+Domain.Name, err.Error())
			}
			glog.Infof("Created Domain: %s", Domain.Name)
		}
	}

	// Initialize local caches

	Zones = make(map[string]*Zone)
	NMGs = make(map[string]*NetworkMacroGroup)
	NMs = make(map[string]*NetworkMacro)

	FreeCIDRs = make(map[string]*net.IPNet)

	// XXX - Create the "FreeCDIRs" map with a predefined (MAX_SUBNETS) nr of per-namespace CIDRs based on the values in the K8S master configuration file
	// The actual VSD Subnets with those prefixes are created on-demand (then removed from this map)

	if err := initCIDRs(conf); err != nil {
		return err
	}

	glog.Info("VSD client initialization completed")
	return nil
}

func GenerateMAC() string {
	buf := make([]byte, 6)
	rand.Seed(time.Now().UTC().UnixNano())
	rand.Read(buf)
	// Set the local bit -- Note the setting of the local bit which means it won't clash with any globally administered addresses (see wikipedia for more info)
	// XXX -- This does _NOT_ work for Nuage VSD
	// buf[0] |= 2
	// XXX - For Nuage VSD
	buf[0] = buf[0]&0xFE | 0x02
	return fmt.Sprintf("%02x:%02x:%02x:%02x:%02x:%02x", buf[0], buf[1], buf[2], buf[3], buf[4], buf[5])
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
func readK8Sconfig(conf *config.AgentConfig) error {
	if data, err := ioutil.ReadFile(conf.MasterConfigFile); err != nil {
		return bambou.NewBambouError("Cannot read K8S Master configuration file: "+conf.MasterConfigFile, err.Error())
	} else {
		if err := yaml.Unmarshal(data, &k8sMasterConfig); err != nil {
			return bambou.NewBambouError("Cannot parse K8S Master configuration file: "+conf.MasterConfigFile, err.Error())
		}
	}
	return nil
}

// Create a connection to the VSD using X.509 certificate-based authentication
func makeX509conn(conf *config.AgentConfig) error {
	if cert, err := tls.LoadX509KeyPair(conf.VsdConfig.CertFile, conf.VsdConfig.KeyFile); err != nil {
		return err
	} else {
		mysession, root = vspk.NewX509Session(&cert, conf.VsdConfig.VsdUrl)
	}

	// mysession.SetInsecureSkipVerify(true)

	if err := mysession.Start(); err != nil {
		return err
	}

	glog.Infof("vsd-client: Successfully established a connection to the VSD at URL is: %s\n", conf.VsdConfig.VsdUrl)

	// glog.Infof("vsd-client: Successfuly established bambou session: %#v\n", *mysession)

	return nil
}

//// Initialize the "FreeCIDRs" map with up to MAX_SUBNETS number of prefixes, based on the values of "ClusterCIDR" and "SubnetLength" (sanity checked)
func initCIDRs(conf *config.AgentConfig) error {
	var err error
	var ccidr *net.IPNet

	if _, ccidr, err = net.ParseCIDR(k8sMasterConfig.NetworkConfig.ClusterCIDR); err != nil {
		return bambou.NewBambouError("Cannot parse K8S cluster network configuration: "+k8sMasterConfig.NetworkConfig.ClusterCIDR, err.Error())

	}
	glog.Infof("K8S master configuration: %#v", k8sMasterConfig)
	glog.Infof("Pod cluster CIDR prefix: %s", ccidr.String())
	cmask, _ := ccidr.Mask.Size() // Nr bits in the ClusterCIDR prefix mask

	// The resulting subnet mask length for the Pod Subnets in the cluster
	smask := uint(cmask + k8sMasterConfig.NetworkConfig.SubnetLength)

	if smask >= 32 {
		glog.Errorf("Invalid resulting subnet mask length for Pod networks: /%d", smask)
	}

	//////// Intialize "FreeCIDRs" map. Values:
	//////// - Nr Subnets: 1<<SubnetLength  (limited to MAX_SUBNETS)
	//////// - Nr hosts per subnet: 1<<(32-smask)  (incl net addr + broadcast)
	////////
	//////// Easiest way to generate the subnet prefixes is to convert them to/from int32 in "nr hosts per subnet" increments

	for i := 0; i < 1<<uint(k8sMasterConfig.NetworkConfig.SubnetLength) && i < MAX_SUBNETS; i++ {
		newprefix := intToIP(ipToInt(ccidr.IP) + int32(i*(1<<(32-smask))))
		FreeCIDRs[newprefix.String()] = &net.IPNet{newprefix, net.CIDRMask(int(smask), 32)}
		// glog.Infof("=> Generated Subnet Prefix: %s", FreeCIDRs[newprefix.String()].String())

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

// XXX - Due to VSD create operations delays, simultaneous create operations may fail with "already exists" (particularly at startup).
// Here we check if the underlying error contains that string (as all "go-bambou" errors of this type should)

func alreadyexistserr(err error) bool {
	if be, ok := err.(*bambou.Error); ok {
		return strings.Contains(be.Description, "already exists")
	}
	return false
}
