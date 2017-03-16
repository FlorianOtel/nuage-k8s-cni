package vsd

import (
	"crypto/tls"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"

	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"

	yaml "gopkg.in/yaml.v2"

	"github.com/golang/glog"

	"github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/config"

	"github.com/nuagenetworks/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

// Wrapper around vspk.Subnet, with custom IPAM
type Subnet struct {
	*vspk.Subnet                    // VSD Subnet. 1-1 mapping (transparent)
	Range        *ipallocator.Range // IPAM for this Subnet
	Customed     bool               // If the net is part of ClusterCIDR or a customed network
}

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
	masterconfig config.MasterConfig

	// Nuage Enterprise and Domain for this K8S cluster. Created if they don't exist already
	Enterprise *vspk.Enterprise
	Domain     *vspk.Domain

	//// XXX - VSD view of things. Must be reconciled with K8S data
	Zones map[string]*vspk.Zone              // Key: ZONE_NAME + Name
	NMGs  map[string]*vspk.NetworkMacroGroup // Key: NMG_NAME + Name
	NMs   map[string]*vspk.EnterpriseNetwork // Key: NM_NAME + Name

	vsdmutex sync.Mutex // Serialize VSD operations, esp creates/updates

	// (Sub)set of allowed prefixes for Pod subnets w
	FreeCIDRs map[string]*net.IPNet // Key: (!!) vspk.Subnet.Address (subnet prefix as string)
)

func InitClient(conf *config.AgentConfig) error {
	if err := readnetconfig(conf); err != nil {
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
			jsonorg, _ := json.MarshalIndent(Enterprise, "", "\t")
			glog.Infof("Found Enterprise Name %#s:\n%#s", Enterprise.Name, string(jsonorg))
		case 0:
			glog.Infof("VSD Enterprise %#s not found, creating...", conf.VsdConfig.Enterprise)
			Enterprise = new(vspk.Enterprise)
			Enterprise.Name = conf.VsdConfig.Enterprise
			Enterprise.Description = "Automatically created Enterprise for K8S Cluster"
			if err := root.CreateEnterprise(Enterprise); err != nil {
				return bambou.NewBambouError("Cannot create Enterprise: "+Enterprise.Name, err.Error())
			}
			jsonorg, _ := json.MarshalIndent(Enterprise, "", "\t")
			glog.Infof("Created Enterprise Name %#s:\n%#s\n", Enterprise.Name, string(jsonorg))
		}
	}

	////  VSD Domain
	if dl, err := root.Domains(&bambou.FetchingInfo{Filter: "name == \"" + conf.VsdConfig.Domain + "\""}); err != nil {
		return bambou.NewBambouError("Error fetching list of Domains from the VSD", err.Error())
	} else {
		switch len(dl) {
		case 1: // Given Domain already exists
			Domain = dl[0]
			jsond, _ := json.MarshalIndent(Domain, "", "\t")
			glog.Infof("Found Domain Name %#s\n%#s", Domain.Name, string(jsond))
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
			jsond, _ := json.MarshalIndent(Domain, "", "\t")
			glog.Infof("Created Domain Name %#s\n%#s", Domain.Name, string(jsond))
		}
	}

	// Initialize local caches

	Zones = make(map[string]*vspk.Zone)
	NMGs = make(map[string]*vspk.NetworkMacroGroup)
	NMs = make(map[string]*vspk.EnterpriseNetwork)

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

/////////
///////// "Check" / 'Create" VSD entities
/////////

////
//// TO DO: Refactor this in its own module, with those functions as methods.
////

// NetworkMacro (Enterprise Network)
func ExistsNM(name string) (*vspk.EnterpriseNetwork, error) {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// First, check the local cache of VSD constructs. If it's there already, return it from the cache
	if nm, exists := NMs[name]; exists {
		glog.Infof("VSD Network Macro with name: %s already cached", nm.Name)
		return nm, nil
	}

	// Second, check the VSD. If it's there, update the local cache and return it
	nms, err := Enterprise.EnterpriseNetworks(&bambou.FetchingInfo{Filter: "name == \"" + name + "\""})
	if err != nil {
		return nil, bambou.NewBambouError("Error fetching list of Network Macros from the VSD", err.Error())
	}

	if len(nms) == 1 {
		glog.Infof("VSD Network Macro with name: %s found on VSD, caching ...", name)
		NMs[name] = nms[0]
		return nms[0], nil
	}

	return nil, nil
}

func CreateNM(nm *vspk.EnterpriseNetwork) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := Enterprise.CreateEnterpriseNetwork(nm); err != nil {
		return bambou.NewBambouError("Cannot create Network Macro: "+nm.Name, err.Error())
	}
	// Add it to the local cache as well.
	// XXX - Up to the caller to ensure there are no map conflicts
	NMs[nm.Name] = nm
	glog.Infof("Successfully created Network Macro: %s", nm.Name)
	return nil
}

// Network Macro Groups
func ExistsNMG(name string) (*vspk.NetworkMacroGroup, error) {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// First, check the local cache of VSD constructs. If it's there already, return it from the cache
	if nmg, exists := NMGs[name]; exists {
		glog.Infof("VSD Network Macro Group with name: %s is already cached", nmg.Name)
		return nmg, nil
	}
	// Second, check the VSD. If it's there, update the local cache and return it
	nmgs, err := Enterprise.NetworkMacroGroups(&bambou.FetchingInfo{Filter: "name == \"" + name + "\""})
	if err != nil {
		return nil, bambou.NewBambouError("Error fetching list of Network Macro Groups from the VSD", err.Error())
	}

	if len(nmgs) == 1 {
		glog.Infof("VSD Network Macro Group with name: %s found on VSD, caching ...", name)
		NMGs[name] = nmgs[0]
		return nmgs[0], nil
	}

	return nil, nil
}

func CreateNMG(nmg *vspk.NetworkMacroGroup) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := Enterprise.CreateNetworkMacroGroup(nmg); err != nil {
		return bambou.NewBambouError("Cannot create Network Macro Group: "+nmg.Name, err.Error())
	}

	// Add it to the local cache as well.
	// XXX - Up to the caller to ensure there are no map conflicts
	NMGs[nmg.Name] = nmg
	glog.Infof("Successfully created Network Macro Group: %s", nmg.Name)
	return nil
}

// Add a Network Macro to a Network Macro Group
func AddNMtoNMG(nm *vspk.EnterpriseNetwork, nmg *vspk.NetworkMacroGroup) error {
	// vsdmutex.Lock()
	// defer vsdmutex.Unlock()

	nmchildren := []*vspk.NetworkMacroGroup{nmg}
	if err := nm.AssignNetworkMacroGroups(nmchildren); err != nil {
		return bambou.NewBambouError("Cannot add Network Macro: "+nm.Name+" to Network Macro Group: "+nmg.Name, err.Error())
	}

	glog.Infof("Successfully added Network Macro: %s to Network Macro Group: %s", nm.Name, nmg.Name)
	return nil
}

// Zones
func ExistsZone(name string) (*vspk.Zone, error) {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// First, check the local cache of VSD constructs. If it's there already, return it from the cache
	if zone, exists := Zones[name]; exists {
		glog.Infof("VSD Zone with name: %s already cached", zone.Name)
		return zone, nil
	}

	// Second, check the VSD. If it's there, update the local cache and return it
	zonelist, err := Domain.Zones(&bambou.FetchingInfo{Filter: "name == \"" + name + "\""})

	if err != nil {
		return nil, bambou.NewBambouError("Cannot fetch Zone: "+name, err.Error())
	}

	if len(zonelist) == 1 {
		glog.Infof("Zone with name: %s found on VSD, caching ...", name)
		Zones[name] = zonelist[0]
		return zonelist[0], nil
	}

	return nil, nil
}

func CreateZone(zone *vspk.Zone) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := Domain.CreateZone(zone); err != nil {
		return bambou.NewBambouError("Cannot create Zone: "+zone.Name, err.Error())
	}
	// Add it to the local cache as well.
	// XXX - Up to the caller to ensure there are no map conflicts
	Zones[zone.Name] = zone
	glog.Infof("Successfully created Zone: %s", zone.Name)
	return nil
}

// Get a list of "Subnet" (*vspk.Subnet + iprange Allocator + customed flag) for a zone
// XXX - IPAM Side effect: Allocate the IP addresses for _containers_ on this subnet.
func ZoneSubnets(zone *vspk.Zone) ([]Subnet, error) {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	var resp []Subnet

	sl, err := zone.Subnets(&bambou.FetchingInfo{})

	if err != nil {
		return nil, bambou.NewBambouError("Cannot fetch list of Subnets for Zone: "+zone.Name, err.Error())
	}

	var subnet Subnet
	for _, s := range sl {

		scidr := net.IPNet{net.ParseIP(s.Address).To4(), net.IPMask(net.ParseIP(s.Netmask).To4())}
		// Remove this prefix from the list of FreeCIDRs, if it was previously available
		if _, wasfree := FreeCIDRs[s.Address]; wasfree {
			glog.Infof("Subnet: %s. Subnet prefix: %s is part of ClusterCIDR address space. Reserving subnet address range...", s.Name, scidr.String())
			subnet.Customed = false
			delete(FreeCIDRs, s.Address)
		} else {
			// Flag it as a custom network
			subnet.Customed = true
			glog.Infof("Custom subnet range: %s found. Reserving subnet address range...", scidr.String())
		}
		subnet.Subnet = s
		// Create a new ipallocator for this subnet
		subnet.Range = ipallocator.NewCIDRRange(&scidr)
		resp = append(resp, subnet)

		// Get the list of all the __container__ endpoints on this subnet
		// XXX - Notes
		// - There may be other entities (other than containers) in this subnet. We ignore those -> potential conflict
		// - No clean way of getting all the endpoints with an IP address in this subnet

		cifaces, _ := s.ContainerInterfaces(&bambou.FetchingInfo{})
		glog.Infof("Found: %d container interfaces in subnet range: %s . Reserving subnet address range...", len(cifaces), scidr.String())

		for _, cif := range cifaces {
			if err := subnet.Range.Allocate(net.ParseIP(cif.IPAddress).To4()); err != nil {
				glog.Errorf("--> Cannot allocate IP address: %s from subnet range: %s . Error: %s", cif.IPAddress, scidr.String(), err)
			}
		}
	}

	return resp, nil
}

// Add Subnet to a Zone
func ZoneAddSubnet(zone *vspk.Zone, s Subnet) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := zone.CreateSubnet(s.Subnet); err != nil {
		return bambou.NewBambouError("Zone: "+zone.Name+" cannot add Subnet: Name: "+s.Subnet.Name+" , Address: "+s.Subnet.Address+" , Netmask: "+s.Subnet.Netmask, err.Error())
	}

	glog.Infof("Zone: %s successfully added Subnet: Name: %s , Address: %s , Netmask: %s", zone.Name, s.Subnet.Name, s.Subnet.Address, s.Subnet.Netmask)

	return nil
}

// Containers
func ExistsContainer(name string) (*vspk.Container, error) {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// XXX - We are not locally caching pods (ephemeral constructs)

	// Check the VSD. If it's there, update the local cache and return it
	containerlist, err := Domain.Containers(&bambou.FetchingInfo{Filter: "name == \"" + name + "\""})

	if err != nil {
		return nil, bambou.NewBambouError("Cannot fetch Container with name: "+name, err.Error())
	}

	if len(containerlist) == 1 {
		glog.Infof("Container with name: %s found on VSD", name)
		return containerlist[0], nil
	}

	return nil, nil
}

func CreateContainer(c *vspk.Container) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := root.CreateContainer(c); err != nil {
		return bambou.NewBambouError("Cannot create Container with name: "+c.Name, err.Error())
	}

	return nil
}

func DeleteContainer(c *vspk.Container) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := c.Delete(); err != nil {
		return bambou.NewBambouError("Cannot delete Container with name: "+c.Name, err.Error())
	}

	return nil
}

// XXX - Notes
// - Assumes Container has valid Interface information associated with it.
// - If it has several interfaces, it only uses the first one
// - Workaround SDK bug: "vspk.Container.Interfaces" is not "[]ContainerInterface" so it unmarshalls into "map[string]interface{}".  As such we access it as a "map[string]interface{}
// - No need to reach to the VSD, so no need for Mutex locking
func ContainerIPandMask(container *vspk.Container) (string, string) {
	if len(container.Interfaces) != 1 {
		glog.Fatalf("Given container does not have exactly one interface. Container info: %#v", container)
	}

	//XXX - "container.Interfaces[0]" is "map[string]interface{}" (arbitrary JSON object) instead of a "ContainerInterface"
	// We deal with that by JSON marshalling & unmarshalling in the (right) type
	data, _ := json.Marshal(container.Interfaces[0])
	ciface := vspk.ContainerInterface{}
	json.Unmarshal(data, &ciface)
	return ciface.IPAddress, ciface.Netmask
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

	if _, ccidr, err = net.ParseCIDR(masterconfig.NetworkConfig.ClusterCIDR); err != nil {
		return bambou.NewBambouError("Cannot parse K8S cluster network configuration: "+masterconfig.NetworkConfig.ClusterCIDR, err.Error())

	}
	glog.Infof("K8S master configuration: %#v", masterconfig)
	glog.Infof("Pod cluster CIDR prefix: %s", ccidr.String())
	cmask, _ := ccidr.Mask.Size() // Nr bits in the ClusterCIDR prefix mask

	// The resulting subnet mask length for the Pod Subnets in the cluster
	smask := uint(cmask + masterconfig.NetworkConfig.SubnetLength)

	if smask >= 32 {
		glog.Errorf("Invalid resulting subnet mask length for Pod networks: /%d", smask)
	}

	//////// Intialize "FreeCIDRs" map. Values:
	//////// - Nr Subnets: 1<<SubnetLength  (limited to MAX_SUBNETS)
	//////// - Nr hosts per subnet: 1<<(32-smask)  (incl net addr + broadcast)
	////////
	//////// Easiest way to generate the subnet prefixes is to convert them to/from int32 in "nr hosts per subnet" increments

	for i := 0; i < 1<<uint(masterconfig.NetworkConfig.SubnetLength) && i < MAX_SUBNETS; i++ {
		newprefix := intToIP(ipToInt(ccidr.IP) + int32(i*(1<<(32-smask))))
		FreeCIDRs[newprefix.String()] = &net.IPNet{newprefix, net.CIDRMask(int(smask), 32)}
		glog.Infof("=> Generated Subnet Prefix: %s", FreeCIDRs[newprefix.String()].String())

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
