package vsd

import (
	"encoding/json"

	"github.com/golang/glog"

	"github.com/nuagenetworks/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

// XXX -- All those methods rely on a configured VSD connection:
// - "root" object
// - valid "Enterprise" and "Domain" set

func (container *Container) Exists(name string) error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	// XXX - We are not locally caching pods (ephemeral constructs)

	// Check the VSD. If it's there, update the local cache and return it
	containerlist, err := Domain.Containers(&bambou.FetchingInfo{Filter: "name == \"" + name + "\""})

	if err != nil {
		return bambou.NewBambouError("Cannot fetch Container with name: "+name, err.Error())
	}

	if len(containerlist) == 1 {
		glog.Infof("Container with name: %s found on VSD", name)
		*container = (Container)(*containerlist[0])
	}

	return nil
}

func (container *Container) Create() error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := root.CreateContainer((*vspk.Container)(container)); err != nil {
		return bambou.NewBambouError("Cannot create Container with name: "+container.Name, err.Error())
	}

	glog.Infof("Container with name: %s created on the VSD", container.Name)
	return nil
}

func (container *Container) Delete() error {
	vsdmutex.Lock()
	defer vsdmutex.Unlock()

	if err := (*vspk.Container)(container).Delete(); err != nil {
		return bambou.NewBambouError("Cannot delete Container with name: "+container.Name, err.Error())
	}

	glog.Infof("Container with name: %s successfully deleted from the VSD", container.Name)
	return nil
}

// XXX - Notes
// - Assumes Container has valid Interface information associated with it.
// - If it has several interfaces, it only uses the first one
// - Workaround SDK bug: "vspk.Container.Interfaces" is not "[]ContainerInterface" so it unmarshalls into "map[string]interface{}".  As such we access it as a "map[string]interface{}
// - No need to reach to the VSD, so no need for Mutex locking
func (container *Container) IPandMask() (string, string) {
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
