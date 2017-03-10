package k8s

import (
	vsd "github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/vsd-client"
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	"github.com/FlorianOtel/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

/////
///// K8S Service <-> VSD NetworkMacro.
///// Coresponding: VSD hierarcy:  K8S Service == VSD NetworkMacro (vspk.EnterpriseNetwork) -> NetworkMacroGroup -> Enterprise
/////

func ServiceCreated(svc *apiv1.Service) error {
	////
	//// Check VSD construct hierachy, bottom up
	////

	// First, check if NM exists
	nm, err := vsd.ExistsNM(vsd.NM_NAME + svc.ObjectMeta.Name)

	if err != nil {
		return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
	}

	if nm == nil {
		glog.Infof("Cannot find VSD Network Macro with name: %s, creating...", vsd.NM_NAME+svc.ObjectMeta.Name)

		// Check if parent NMG (all services in same K8S namespace) exists
		nmg, err := vsd.ExistsNMG(vsd.NMG_NAME + svc.ObjectMeta.Namespace)
		if err != nil {
			return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
		}

		if nmg == nil {
			glog.Infof("Cannot find a VSD Network Macro Group with name: %s, creating...", vsd.NMG_NAME+svc.ObjectMeta.Namespace)

			// Create a new NMG
			nmg = new(vspk.NetworkMacroGroup)
			nmg.Name = vsd.NMG_NAME + svc.ObjectMeta.Namespace
			if err := vsd.CreateNMG(nmg); err != nil {
				return err
			}
		}

		// Create a new NM under this NMG (prev existing or just created)
		nm = new(vspk.EnterpriseNetwork)
		nm.Name = vsd.NM_NAME + svc.ObjectMeta.Name
		//NM Address is the Service IP address. Netmask is "255.255.255.255"
		nm.Address = svc.Spec.ClusterIP
		nm.Netmask = "255.255.255.255"
		if err := vsd.CreateNM(nm); err != nil {
			return err
		}

		if err := vsd.AddNMtoNMG(nm, nmg); err != nil {
			return err
		}

	}
	// Still TBD if need this extra layer of caching  -- add the Service to the list of K8S services
	// Services[svc.ObjectMeta.Name] = nm
	return nil

}

func ServiceDeleted(svc *apiv1.Service) error {
	glog.Info("=====> A service got deleted")
	JsonPrettyPrint("service", svc)
	return nil
}

// Still TBD if / when / how to use  -- stub so far
func ServiceUpdated(old, updated *apiv1.Service) error {

	return nil
}
