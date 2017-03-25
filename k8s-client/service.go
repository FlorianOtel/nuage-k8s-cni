package k8s

import (
	vsdclient "github.com/OpenPlatformSDN/nuage-k8s-cni/vsd-client"
	"github.com/golang/glog"
	//

	"github.com/FlorianOtel/go-bambou/bambou"
	apiv1 "github.com/OpenPlatformSDN/client-go/pkg/api/v1"
)

/////
///// K8S Service <-> VSD NetworkMacro.
///// Coresponding: VSD hierarcy:  K8S Service == VSD NetworkMacro (vspk.EnterpriseNetwork) -> NetworkMacroGroup -> Enterprise
/////

func ServiceCreated(svc *apiv1.Service) error {
	////
	//// Check VSD construct hierachy, bottom up
	////

	nm := new(vsdclient.NetworkMacro)
	nm.Name = vsdclient.NM_NAME + svc.ObjectMeta.Name

	// First, check if NM exists
	if err := nm.FetchByName(); err != nil {
		return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
	}

	if nm.ID == "" { // Couldn't find it
		glog.Infof("Cannot find VSD Network Macro with name: %s, creating...", nm.Name)

		// Check if parent NMG (all services in same K8S namespace) exists
		nmg := new(vsdclient.NetworkMacroGroup)
		nmg.Name = vsdclient.NMG_NAME + svc.ObjectMeta.Namespace

		if err := nmg.FetchByName(); err != nil {
			return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
		}

		if nmg.ID == "" {
			glog.Infof("Cannot find a VSD Network Macro Group with name: %s, creating...", nmg.Name)
			// Create it
			if err := nmg.Create(); err != nil {
				return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
			}
		}

		// Create the NM under this NMG (prev existing or just created)
		// Name was set above. Address is the Service IP address. Netmask is "255.255.255.255"
		nm.Address = svc.Spec.ClusterIP
		nm.Netmask = "255.255.255.255"
		if err := nm.Create(); err != nil {
			return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
		}

		if err := nmg.AddNM(nm); err != nil {
			return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
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
