package k8s

import (
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	"github.com/FlorianOtel/go-bambou/bambou"
	vsd "github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/vsd-client"
	"github.com/nuagenetworks/vspk-go/vspk"
)

/////
///// K8S Namespace <-> VSD Zone
///// Parent VSD hierarcy: Zone -> Domain
/////
func NamespaceCreated(ns *apiv1.Namespace) error {

	// Chceck if we still have a VSD zone with this name (cached from previous instances of the agent )
	zone, err := vsd.ExistsZone(vsd.ZONE_NAME + ns.ObjectMeta.Name)

	if err != nil {
		return bambou.NewBambouError("Error creating K8S namespace: "+ns.ObjectMeta.Name, err.Error())
	}

	if zone == nil {
		glog.Infof("Cannot find VSD Zone with name: %s, creating...", vsd.ZONE_NAME+ns.ObjectMeta.Name)

		// Create a new VSD zone
		zone = new(vspk.Zone)
		zone.Name = vsd.ZONE_NAME + ns.ObjectMeta.Name
		if err := vsd.CreateZone(zone); err != nil {
			return err
		}
	}

	// Add the zone to the list of K8S namespaces
	Namespaces[ns.ObjectMeta.Name] = namespace{zone, nil}
	return nil

}

func NamespaceDeleted(ns *apiv1.Namespace) error {
	glog.Info("=====> A namespace got deleted")
	JsonPrettyPrint("namespace", ns)
	return nil
}

// Still TBD if / when / how to use  -- stub so far
func NamespaceUpdated(old, updated *apiv1.Namespace) error {
	return nil
}
