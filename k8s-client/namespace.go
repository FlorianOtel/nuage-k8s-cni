package k8s

import (
	"github.com/golang/glog"
	//

	"github.com/FlorianOtel/go-bambou/bambou"
	apiv1 "github.com/OpenPlatformSDN/client-go/pkg/api/v1"
	vsdclient "github.com/OpenPlatformSDN/nuage-k8s-cni/vsd-client"
)

////
//// K8S Namespace <-> VSD Zone
//// VSD object hierarcy: Zone -> Domain <== Handled at startup
////  Convention: VSD Zone name = vsdclient.ZONE_NAME + ns.ObjectMeta.Name

func NamespaceCreated(ns *apiv1.Namespace) error {

	zone := new(vsdclient.Zone)
	zone.Name = vsdclient.ZONE_NAME + ns.ObjectMeta.Name

	// Chceck if we still have a VSD zone with this name (cached from previous instances of the agent )
	if err := zone.FetchByName(); err != nil {
		return bambou.NewBambouError("Error creating K8S namespace: "+ns.ObjectMeta.Name, err.Error())
	}

	if zone.ID == "" { // Zone does not exist, create it
		glog.Infof("Cannot find VSD Zone with name: %s, creating...", zone.Name)
		if err := zone.Create(); err != nil {
			return err
		}

		////
		//// Still TBD -- Insert logic here if this K8S namespace is created with e.g. custom subnets
		////

	}

	// Get the list of Subnets (ranges + ipallocator's) for this zone (if any)
	nssubnets, _ := zone.Subnets()

	// Add it to the list of K8S namespaces
	Namespaces[ns.ObjectMeta.Name] = namespace{zone, nssubnets}

	// glog.Info("=====> A namespace got created")
	// JsonPrettyPrint("namespace", ns)

	return nil

}

func NamespaceDeleted(ns *apiv1.Namespace) error {
	//
	// Insert logic here
	//

	glog.Info("=====> A namespace got deleted")
	JsonPrettyPrint("namespace", ns)
	return nil
}

// Still TBD if / when / how to use  -- stub so far
func NamespaceUpdated(old, updated *apiv1.Namespace) error {
	//
	// Insert logic here
	//
	return nil
}

//
func NamespaceNOP(ns *apiv1.Namespace) error {
	select {}
}
