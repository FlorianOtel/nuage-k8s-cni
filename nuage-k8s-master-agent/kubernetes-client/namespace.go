package k8s

import (
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	"github.com/FlorianOtel/go-bambou/bambou"
	vsd "github.com/OpenPlatformSDN/nuage-cni-k8s/nuage-k8s-master-agent/vsd-client"
	"github.com/nuagenetworks/vspk-go/vspk"
)

/////
///// K8S Namespace <-> VSD Zone
///// Parent VSD hierarcy: Zone -> Domain
/////
func NamespaceCreated(ns *apiv1.Namespace) error {
	glog.Info("#### Handling namespace creation")
	JsonPrettyPrint("namespace", ns)

	// Chceck if we still have a VSD zone with this name (cached from previous instances of the agent )
	zone, exists := vsd.Zones[vsd.ZONE_NAME+ns.ObjectMeta.Name]

	if !exists {
		// Create a new VSD zone
		zone = new(vspk.Zone)
		zone.Name = vsd.ZONE_NAME + ns.ObjectMeta.Name
		if err := vsd.Domain.CreateZone(zone); err != nil {
			return bambou.NewBambouError("Cannot create VSD Zone: "+vsd.ZONE_NAME+ns.ObjectMeta.Name, err.Error())
		}
		glog.Infof("Created new Zone: %s", vsd.ZONE_NAME+ns.ObjectMeta.Name)
		// Add it to the cached list of VSD Zones
		vsd.Zones[vsd.ZONE_NAME+ns.ObjectMeta.Name] = zone

	} else {
		glog.Infof(" Found existing VSD Zone: %s", vsd.ZONE_NAME+ns.ObjectMeta.Name)
		////
		//// Still TBD ... how to handle old / cached VSD constructs
		////
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
