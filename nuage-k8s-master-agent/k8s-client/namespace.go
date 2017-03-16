package k8s

import (
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	"github.com/FlorianOtel/go-bambou/bambou"
	vsd "github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/vsd-client"
	"github.com/nuagenetworks/vspk-go/vspk"
)

////
//// K8S Namespace <-> VSD Zone
//// VSD object hierarcy: Zone -> Domain <== Handled at startup
////  Convention: VSD Zone name = vsd.ZONE_NAME + ns.ObjectMeta.Name

func NamespaceCreated(ns *apiv1.Namespace) error {

	var nssubnets []vsd.Subnet // List of subnets to be associated with this Namespace, if any found / given

	// Chceck if we still have a VSD zone with this name (cached from previous instances of the agent )
	zone, err := vsd.ExistsZone(vsd.ZONE_NAME + ns.ObjectMeta.Name)

	if err != nil {
		return bambou.NewBambouError("Error creating K8S namespace: "+ns.ObjectMeta.Name, err.Error())
	}

	if zone != nil { //VSD Zone already exists
		// Get the list of Subnets (ranges + ipallocator's) for this zone
		nssubnets, _ = vsd.ZoneSubnets(zone)
	} else { // Zone does not exist, create it
		glog.Infof("Cannot find VSD Zone with name: %s, creating...", vsd.ZONE_NAME+ns.ObjectMeta.Name)

		// Create a new VSD zone
		zone = new(vspk.Zone)
		zone.Name = vsd.ZONE_NAME + ns.ObjectMeta.Name
		if err := vsd.CreateZone(zone); err != nil {
			return err
		}

		////
		//// Still TBD -- Insert logic here if this K8S namespace is created with e.g. custom subnets
		////

	}

	// Check if the VSD zone already has Subnets attached to it

	// Add it to the list of K8S namespaces
	Namespaces.Lock()
	Namespaces.nscache[ns.ObjectMeta.Name] = namespace{zone, nssubnets}
	Namespaces.Unlock()

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
