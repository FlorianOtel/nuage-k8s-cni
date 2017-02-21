package k8s

import (
	vsd "github.com/OpenPlatformSDN/nuage-cni-k8s/nuage-k8s-master-agent/vsd-client"
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	"github.com/FlorianOtel/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
	// "github.com/FlorianOtel/client-go/pkg/util/wait"
)

/////
///// K8S Service <-> VSD NetworkMacro.
///// Parent VSD hierarcy: NetworkMacro -> NetworkMacroGroup -> Enterprise
/////
func ServiceCreated(svc *apiv1.Service) error {
	glog.Info(" ##### Handling service creation")
	JsonPrettyPrint("service", svc)

	// Check VSD construct hierachy, bottom up
	nm, exists := vsd.NMs[vsd.NM_NAME+svc.ObjectMeta.Name]
	// Check if NM exists
	if !exists {
		// Check if parent NMG exists
		if nmg, exists := vsd.NMGs[vsd.NMG_NAME+svc.ObjectMeta.Namespace]; !exists {
			// Create a new NMG
			nmg = new(vspk.NetworkMacroGroup)
			nmg.Name = vsd.NMG_NAME + svc.ObjectMeta.Namespace
			if err := vsd.Enterprise.CreateNetworkMacroGroup(nmg); err != nil {
				return bambou.NewBambouError("Cannot create Network Macro Group: "+nmg.Name, err.Error())
			}
			glog.Infof("Created a new Network Macro Group: %s", nmg.Name)
			// Add it to the cached list of VSD NMGs
			vsd.NMGs[vsd.NMG_NAME+svc.ObjectMeta.Namespace] = nmg
		} else {
			glog.Infof("Found existing VSD Network Macro Group: %s", vsd.NMG_NAME+svc.ObjectMeta.Namespace)
			// Create a new NM
			nm = new(vspk.EnterpriseNetwork)
			nm.Name = vsd.NM_NAME + svc.ObjectMeta.Name
			// NM Address is the Service IP address. Netmask is "255.255.255.255"
			nm.Address = svc.Spec.ClusterIP
			nm.Netmask = "255.255.255.255"
			if err := vsd.Enterprise.CreateEnterpriseNetwork(nm); err != nil {
				return bambou.NewBambouError("Cannot create Network Macro: "+nm.Name, err.Error())
			}
			glog.Infof("Created a new Network Macro: %s", nm.Name)
			vsd.NMs[vsd.NM_NAME+svc.ObjectMeta.Name] = nm
		}

	} else {
		glog.Infof(" Found existing VSD Network Macro: %s", vsd.NM_NAME+svc.ObjectMeta.Name)
		////
		//// Still TBD ... how to handle old / cached VSD constructs
		////
	}

	// Add the Service to the list of K8S services
	Services[svc.ObjectMeta.Name] = nm
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
