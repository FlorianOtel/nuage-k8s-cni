package k8s

import (
	"time"

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
	// Ensure that the Namespace is already created -- due event processing race conditions at startup, service creation event may be processed before namespace creation
	// XXX -- The name is the VSD Zone name (different from the K8s namespace name itself)
	if _, exists := Namespaces[svc.ObjectMeta.Namespace]; !exists {
		// Wait for a max 10 seconds, probing local cache
		timeChan := time.NewTimer(time.Second * 10).C
		tickChan := time.NewTicker(time.Millisecond * 100).C

		for {
			select {
			case <-timeChan:
				return bambou.NewBambouError("Error creating K8S Pod: "+svc.ObjectMeta.Name, "Timeout waiting for namespace "+svc.ObjectMeta.Namespace+" to be created")
			case <-tickChan:
				if _, found := Namespaces[svc.ObjectMeta.Namespace]; !found {
					continue
				}
			}
			break
		}
	}

	////
	//// Check VSD construct hierachy, top down.  Enterprise/Domain are already created
	////
	nm := new(vsdclient.NetworkMacro)
	nm.Name = vsdclient.NM_NAME + svc.ObjectMeta.Name

	// Parent NMG (all services in same K8S namespace)
	nmg := new(vsdclient.NetworkMacroGroup)
	nmg.Name = vsdclient.NMG_NAME + svc.ObjectMeta.Namespace

	// First, check that NMG exists

	if err := nmg.FetchByName(); err != nil {
		return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
	}

	if nmg.ID == "" {
		glog.Infof("Cannot find a VSD Network Macro Group with name: %s, creating...", nmg.Name)
		// Create it
		if err := nmg.Create(); err != nil {
			return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
		}

		// Add a PE (Policy Element) allowing traffic to the services in this namespace (VSD Zone)
		if err := nmg.AddPESvcsAllow(Namespaces[svc.ObjectMeta.Namespace].Zone); err != nil {
			// We might get an error if the NMG was deleted but the Policy Element was still there. Just log the error
			glog.Errorf("Cannot add network Policy Element for K8S Services in namespace %s. Error: %s", svc.ObjectMeta.Namespace, err)
		} else {
			glog.Infof("Added network Policy Element for K8S Services in namespace %s", svc.ObjectMeta.Namespace)
		}
	}

	// Secondly, check if NM exists
	if err := nm.FetchByName(); err != nil {
		return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
	}

	if nm.ID == "" { // Couldn't find it
		glog.Infof("Cannot find VSD Network Macro with name: %s, creating...", nm.Name)

		// Create the NM under the NMG (prev existing or created above)
		// Name was set above. Address is the Service IP address. Netmask is "255.255.255.255"
		nm.Address = svc.Spec.ClusterIP
		nm.Netmask = "255.255.255.255"
		if err := nm.Create(); err != nil {
			return bambou.NewBambouError("Error creating K8S service: "+svc.ObjectMeta.Name, err.Error())
		}
	}

	if err := nmg.AddNM(nm); err != nil { // We might get errors -- e.g. in the case this NM was already added to the NMG. Just log them.
		glog.Errorf("Error creating service: %s. Cannot add NetworkMacro: %s to NetworkMacroGroup: %s . Error: %s", svc.ObjectMeta.Name, nm.Name, nmg.Name, err)
	}

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
