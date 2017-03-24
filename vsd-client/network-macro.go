package vsd

import (
	"github.com/golang/glog"
	"github.com/nuagenetworks/vspk-go/vspk"

	"github.com/nuagenetworks/go-bambou/bambou"
)

// XXX -- All those methods rely on a configured VSD connection:
// - "root" object
// - valid "Enterprise" and "Domain" set

// NetworkMacro (Enterprise Network). Mutates the receiver if it exists
func (nm *NetworkMacro) Exists(name string) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// First, check the local cache of VSD constructs. If it's there already, return it from the cache
	if NMs[name] != nil {
		*nm = *NMs[name]
		glog.Infof("VSD Network Macro with name: %s already cached", nm.Name)
		return nil
	}

	// Second, check the VSD. If it's there, update the local cache and return it
	nmlist, err := Enterprise.EnterpriseNetworks(&bambou.FetchingInfo{Filter: "name == \"" + name + "\""})
	if err != nil {
		return bambou.NewBambouError("Error fetching list of Network Macros from the VSD", err.Error())
	}

	if len(nmlist) == 1 {
		glog.Infof("VSD Network Macro with name: %s found on VSD, caching ...", name)
		NMs[name] = (*NetworkMacro)(nmlist[0])
		*nm = *NMs[name]
	}

	return nil
}

func (nm *NetworkMacro) Create() error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := Enterprise.CreateEnterpriseNetwork((*vspk.EnterpriseNetwork)(nm)); err != nil {
		return bambou.NewBambouError("Cannot create Network Macro: "+nm.Name, err.Error())
	}

	// Add it to the local cache as well.
	// XXX - Up to the caller to ensure there are no map conflicts
	NMs[nm.Name] = nm
	glog.Infof("Successfully created Network Macro: %s", nm.Name)
	return nil
}
