package vsd

import (
	"net"

	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"

	"github.com/golang/glog"

	"github.com/nuagenetworks/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

// XXX -- All those methods rely on a configured VSD connection:
// - "root" object
// - valid "Enterprise" and "Domain" set

func (zone *Zone) Exists(name string) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// First, check the local cache of VSD constructs. If it's there already, return it from the cache
	if Zones[name] != nil {
		*zone = *Zones[name]
		glog.Infof("VSD Zone with name: %s already cached", zone.Name)
		return nil
	}

	// Second, check the VSD. If it's there, update the local cache and return it
	zonelist, err := Domain.Zones(&bambou.FetchingInfo{Filter: "name == \"" + name + "\""})

	if err != nil {
		return bambou.NewBambouError("Cannot fetch Zone: "+name, err.Error())
	}

	if len(zonelist) == 1 {
		glog.Infof("Zone with name: %s found on VSD, caching ...", name)
		Zones[name] = (*Zone)(zonelist[0])
		*zone = *Zones[name]
	}

	return nil
}

func (zone *Zone) Create() error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := Domain.CreateZone((*vspk.Zone)(zone)); err != nil {
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
func (zone *Zone) Subnets() ([]Subnet, error) {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	var resp []Subnet

	sl, err := (*vspk.Zone)(zone).Subnets(&bambou.FetchingInfo{})

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
func (zone *Zone) AddSubnet(s Subnet) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := (*vspk.Zone)(zone).CreateSubnet(s.Subnet); err != nil {
		return bambou.NewBambouError("Zone: "+zone.Name+" cannot add Subnet: Name: "+s.Subnet.Name+" , Address: "+s.Subnet.Address+" , Netmask: "+s.Subnet.Netmask, err.Error())
	}

	glog.Infof("Zone: %s successfully added Subnet: Name: %s , Address: %s , Netmask: %s", zone.Name, s.Subnet.Name, s.Subnet.Address, s.Subnet.Netmask)

	return nil
}
