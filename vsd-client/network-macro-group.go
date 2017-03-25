package vsd

import (
	"github.com/golang/glog"

	"github.com/nuagenetworks/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

// XXX -- All those methods rely on a configured VSD connection:
// - "root" object
// - valid "Enterprise" and "Domain" set

func (nmg *NetworkMacroGroup) FetchByName() error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// First, check the local cache of VSD constructs. If it's there already, return it from the cache
	if NMGs[nmg.Name] != nil {
		*nmg = *NMGs[nmg.Name]
		glog.Infof("VSD Network Macro Group with name: %s is already cached", nmg.Name)
		return nil
	}

	// Second, check the VSD. If it's there, update the local cache and return it

	// XXX - Notes
	// - VSD bug: VSD side filtering like fails for names NMGs for some reason (4.0r4)
	// - As such we go client side filtering -- i.e. get all the list of NMGs and see if there's one for which the names match

	// nmgs, err = Enterprise.NetworkMacroGroups(&bambou.FetchingInfo{Filter: "name == \"" + nmg.Name + "\""})

	nmglist, err := Enterprise.NetworkMacroGroups(&bambou.FetchingInfo{})
	if err != nil {
		return bambou.NewBambouError("Error fetching list of Network Macro Groups from the VSD", err.Error())
	}
	for _, vsdnmg := range nmglist {
		if vsdnmg.Name == nmg.Name {
			glog.Infof("VSD Network Macro Group with name: %s found on VSD, caching ...", nmg.Name)
			NMGs[nmg.Name] = (*NetworkMacroGroup)(vsdnmg)
			*nmg = *NMGs[nmg.Name]
			break
		}
	}

	return nil
}

func (nmg *NetworkMacroGroup) Create() error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := Enterprise.CreateNetworkMacroGroup((*vspk.NetworkMacroGroup)(nmg)); err != nil {
		return bambou.NewBambouError("Cannot create Network Macro Group: "+nmg.Name, err.Error())
	}

	// Add it to the local cache as well.
	// XXX - Up to the caller to ensure there are no map conflicts
	NMGs[nmg.Name] = nmg
	glog.Infof("Successfully created Network Macro Group: %s", nmg.Name)
	return nil
}

// Add a Network Macro to a Network Macro Group
// XXX - no checks performed.
func (nmg *NetworkMacroGroup) AddNM(nm *NetworkMacro) error {
	// vsdmutex.Lock()
	// defer vsdmutex.Unlock()

	nmchildren := []*vspk.NetworkMacroGroup{(*vspk.NetworkMacroGroup)(nmg)}
	if err := (*vspk.EnterpriseNetwork)(nm).AssignNetworkMacroGroups(nmchildren); err != nil {
		return bambou.NewBambouError("Cannot add Network Macro: "+nm.Name+" to Network Macro Group: "+nmg.Name, err.Error())
	}

	glog.Infof("Successfully added Network Macro: %s to Network Macro Group: %s", nm.Name, nmg.Name)
	return nil
}
