package k8s

import (
	"fmt"
	"net"
	"strings"
	"time"

	cniagent "github.com/OpenPlatformSDN/cni-plugin/nuage-cni-agent/client"

	cniclient "github.com/OpenPlatformSDN/nuage-k8s-cni/cni-agent-client"
	vsdclient "github.com/OpenPlatformSDN/nuage-k8s-cni/vsd-client"
	"github.com/golang/glog"
	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"

	apiv1 "github.com/OpenPlatformSDN/client-go/pkg/api/v1"
	"github.com/nuagenetworks/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

////
//// K8S Pod <-> VSD Container
//// VSD hierarcy: Pod -> Subnet -> Zone (K8S Namespace) <== Handled by Namespace handling
////
////Important conventions (used by CNI plugin):
// - VSD container name = <pod.ObjectMeta.Name>_<pod.ObjectMeta.Namespace>   (VSD container names need to be unique across the whole domain)
// - VSD Container UUID (256 bits, Docker UUID)  ~=  K8S UID, doubled, with dashes removed
// - VSD Container OrchestrationID is "Kubernetes"

func PodCreated(pod *apiv1.Pod) error {

	// Ensure that the pod Namespace is already created -- due event processing race conditions at startup, pod creation event may be processed before namespace creation
	// XXX -- The name is the VSD Zone name (different from the K8s namespace name itself)
	if _, exists := Namespaces[pod.ObjectMeta.Namespace]; !exists {
		// Wait for a max 10 seconds, probing local cache
		timeChan := time.NewTimer(time.Second * 10).C
		tickChan := time.NewTicker(time.Millisecond * 100).C

		for {
			select {
			case <-timeChan:
				return bambou.NewBambouError("Error creating K8S Pod: "+pod.ObjectMeta.Name, "Timeout waiting for namespace "+pod.ObjectMeta.Namespace+" to be created")
			case <-tickChan:
				if _, found := Namespaces[pod.ObjectMeta.Namespace]; !found {
					continue
				}
			}
			break
		}
	}

	// XXX -- at this point "Namespaces[pod.ObjectMeta.Namespace]" points to a valid "namespace"

	/////
	///// Get pod networking details.
	/////

	// Case 1: Pod already has a VSD container associated with it (agent startup, previously existing pod)
	if container, err := case1create(pod); err == nil {
		if container != nil {
			return nil
		}
	}

	//
	// Case 2: Custom settings pod -- custom network settings (custom subnet / ip addr) etc -- via "nuage.io" labels
	//
	if container, err := case2create(pod); err == nil {
		if container != nil {
			return nil
		}
	}

	// Case 3: "Normal" pod --  Allocate an IP address from a non-custom subnet (subnet from ClusterCIDR address space).
	// Allocate a non-custom subnet if none exists previously  / no free IP address are available in any of those subnets
	if container, err := case3create(pod); err == nil {
		if container != nil {
			return nil
		}
	}

	return nil

}

func PodDeleted(pod *apiv1.Pod) error {
	// Do _NOT_ change those conventions -- the CNI agent relies on them.
	// Container Name
	cName := pod.ObjectMeta.Name + "_" + pod.ObjectMeta.Namespace
	// Container UUID
	//cUUID := strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1) + strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1)

	// XXX - Since the bottom part of the plugin removes the VRS entity, the container may or may not be in the VSD at this time
	// As such we pick up the container from the CNI Agent server running on pod's node

	var container *vsdclient.Container

	if c, err := cniagent.ContainerGET(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, cName); err != nil {
		glog.Errorf("Deleting K8S Pod: %s . Cannot fecth container: %s from CNI Agent server on host: %s. Error: %s", pod.ObjectMeta.Name, cName, pod.Spec.NodeName, err.Error())
		return err
	} else {
		container = (*vsdclient.Container)(c)
	}

	// Get the IP address of the container
	cIPv4Addr, cIPv4Mask := container.IPandMask()
	cifaddr := net.ParseIP(cIPv4Addr).To4()

	// Get the subnet address for this IP address, as a string
	sprefix := cifaddr.Mask(net.IPMask(net.ParseIP(cIPv4Mask).To4())).String()

	// Find the subnet in pod's Namespace where this pod was located, and release its IP address from that subnet

	// found := false
	for _, subnet := range Namespaces[pod.ObjectMeta.Namespace].Subnets {
		if sprefix == subnet.Subnet.Address {
			if err := subnet.Range.Release(cifaddr); err != nil {
				glog.Errorf("Deleting K8S pod: %s. Failed to deallocate pod's IP address: %s from Subnet: %s . Error: %s", pod.ObjectMeta.Name, cIPv4Addr, subnet.Subnet.Name, err)
			} else {
				glog.Infof("Deleting K8S pod: %s. Deallocated pod's IP address: %s from Subnet: %s", pod.ObjectMeta.Name, cIPv4Addr, subnet.Subnet.Name)
				// found = true
				break
			}
		}
	}

	// Uncomment this if ip address deallocation has issues
	/*
		if !found {
			glog.Errorf("---> Error deleting K8S pod: %s. Failed to deallocate pod's IP address: %s from prefix: %s. Subnet not found..", pod.ObjectMeta.Name, cIPv4Addr, sprefix)
			for _, s := range Namespaces.nscache[pod.ObjectMeta.Namespace].Subnets {
				glog.Errorf("---> Namespace subnet: Name: %s . Address: %s . Customed: %v", s.Subnet.Name, s.Subnet.Address, s.Customed)
			}
		}
	*/

	// Remove Nuage container from agent server container cache -- ignore any errors
	cniagent.ContainerDELETE(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, cName)

	// Container is deleted from the VSD by the CNI plugin on the node
	return nil
}

func PodUpdated(old, updated *apiv1.Pod) error {

	////
	//// XXX - Still TBD: What scenarios of pod updates we cover -> most certainly the code below will need refactoring
	////

	// Do _NOT_ change those conventions -- the CNI agent relies on them.
	// XXX  -- Use orginal (i.e. "old" pod) values

	// Container Name
	cName := old.ObjectMeta.Name + "_" + old.ObjectMeta.Namespace
	// Container UUID
	// cUUID := strings.Replace(string(old.ObjectMeta.UID), "-", "", -1) + strings.Replace(string(old.ObjectMeta.UID), "-", "", -1)

	//
	// Case: Newly created pod is scheduled on a specific node
	// Action: Post the vspk.Container to the CNI Agent on the scheduled node.

	if (old.Spec.NodeName == "") && (updated.Spec.NodeName != "") {
		if container, exists := Pods[cName]; exists { // This pod is in the "Pods" cache, submitted at creation
			// Post it to the CNI Agent server on the scheduled node and remove it from the cache
			glog.Infof("K8S pod: %s. Scheduled to run on host: %s. Notifying CNI Agent server on that node...", old.ObjectMeta.Name, updated.Spec.NodeName)
			if err := cniagent.ContainerPUT(cniclient.AgentClient, updated.Spec.NodeName, cniclient.AgentServerPort, (*vspk.Container)(container)); err != nil {
				glog.Errorf("Updating K8S pod: %s. Failed to submit VSD container: %s to CNI Agent server on host: %s . Error: %s", old.ObjectMeta.Name, container.Name, updated.Spec.NodeName, err)
				return err
			}
			delete(Pods, cName)
		}
	}

	// glog.Info("=====> A pod got UPDATED")
	// glog.Info("=====> Old pod:")
	// JsonPrettyPrint("pod", old)
	// glog.Info("=====> Updated pod:")
	// JsonPrettyPrint("pod", updated)
	return nil
}

///// Auxilary functions

//// XXX - The return *vspk.Container, in non-nil, _must_ contain a "vspk.ContainerInterface" properly configured

// Case 1: Pod already has a VSD container associated with it (agent startup, previously existing pod)
func case1create(pod *apiv1.Pod) (*vsdclient.Container, error) {
	// Do _NOT_ change those conventions -- the CNI agent relies on them.
	// Container Name
	cName := pod.ObjectMeta.Name + "_" + pod.ObjectMeta.Namespace
	// Container UUID
	cUUID := strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1) + strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1)

	container := new(vsdclient.Container)

	if err := container.Exists(cName); err != nil {
		return nil, bambou.NewBambouError("Error creating K8S Pod: "+pod.ObjectMeta.Name, err.Error())
	}

	if container.Name != "" { // Found it
		cIPv4Addr, cIPv4Mask := container.IPandMask()
		// XXX - No need to handle IPAM here. The container interface was allocated when we parsed the corresponding Subnet
		cifaddr := net.IPNet{net.ParseIP(cIPv4Addr).To4(), net.IPMask(net.ParseIP(cIPv4Mask).To4())}
		glog.Infof("Creating K8S pod: %s already created. VSD container details: Name: %s . UUID: %s . IP address: %s", pod.ObjectMeta.Name, container.Name, cUUID, cifaddr.String())

		// XXX - At startup previously existing pods have a valid "pod.Spec.NodeName", so this error checking is a bit overkill
		if err := cniagent.ContainerPUT(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, (*vspk.Container)(container)); err == nil {
			glog.Infof("Creating K8S pod: %s . Successfully submitted VSD container: %s to CNI Agent server on host: %s", pod.ObjectMeta.Name, container.Name, pod.Spec.NodeName)
		}

		return container, nil
	}

	return nil, nil
}

//
// Case 2: Custom settings pod -- custom network settings (custom subnet / ip addr) etc -- via "nuage.io" labels
//
// Examples:
// "nuage.io/Subnet = <subnet_name>"
// "nuage.io/IPAddress = <ipaddr>"
// "nuage.io/PolicyGroup=<pg>"
// .....
func case2create(pod *apiv1.Pod) (*vsdclient.Container, error) {
	// Do _NOT_ change those conventions -- the CNI agent relies on them.
	// Container Name
	cName := pod.ObjectMeta.Name + "_" + pod.ObjectMeta.Namespace
	// Container UUID
	cUUID := strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1) + strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1)
	// XXX - Above we made sure this is not nil (VSD Zone is created)
	podNsZone := Namespaces[pod.ObjectMeta.Namespace]

	// Check if we got any custom labels.

	if len(pod.ObjectMeta.Labels) == 0 {
		return nil, nil
	}

	// K-V pairs of of "nuage.io" settings
	nuageio := make(map[string]string)

	for label, value := range pod.ObjectMeta.Labels {
		key := strings.SplitAfterN(label, "nuage.io/", 2) // e.g. "nuage.io/Subnet"
		if (len(key) == 2) && (key[1] != "") {
			nuageio[key[1]] = value // nuageio["Subnet"] = <value>
		}

	}

	// We got custom labels but none was of interest
	if len(nuageio) == 0 {
		return nil, nil
	}

	glog.Infof("Creating K8S pod: %s . Custom Nuage labels identified: %v", pod.ObjectMeta.Name, nuageio)

	// Label parsing logic

	// Container interface IP address
	var cifaddr *net.IP
	// Container subnet
	var csubnet *vsdclient.Subnet

	for key, value := range nuageio {
		switch key {
		case "Subnet": // Try to find a "Customed" subnet with this name
			for _, subnet := range podNsZone.Subnets {
				if !subnet.Customed {
					continue
				}
				if subnet.Subnet.Name == value {
					// Found custom subnet with this name. Save it
					csubnet = &subnet
					break
				}
			}
		case "IPAddress":
			if validip := net.ParseIP(value).To4(); validip != nil {
				cifaddr = &validip
			}
		case "PolicyGroup":
			/*
				Insert logic for processing custom labels / Policy Groups
			*/
		}
	}

	// Sanity checking
	if csubnet == nil { // Custom address given but no subnet information
		err := fmt.Errorf("Creating K8S pod: %s . No matching custom subnet name found", pod.ObjectMeta.Name)
		glog.Error(err)
		return nil, err
	}

	// If an IPAddress was given, try to allocate it. If not, try to allocate a new one on the given subnet
	if cifaddr == nil {
		if allocd, err := csubnet.Range.AllocateNext(); err != nil { // Cannot allocate an IP address on this subnet
			err = fmt.Errorf("Creating K8S pod: %s . Cannot allocate an IPv4 Address on Subnet: %s . Error: %s", pod.ObjectMeta.Name, csubnet.Subnet.Name, err)
			glog.Error(err)
			return nil, err
		} else {
			// Successfully allocated an ip adress on this subnet. Save it.
			cifaddr = &allocd
		}
	} else { // Custom IP address given, try to allocate it.
		if err := csubnet.Range.Allocate(*cifaddr); err != nil { // Cannot allocate this IP address
			err = fmt.Errorf("Creating K8S pod: %s . Cannot allocate given IPv4 Address: %s on Subnet: %s . Error: %s", pod.ObjectMeta.Name, cifaddr.String(), csubnet.Subnet.Name, err)
			glog.Error(err)
			return nil, err
		}
	}

	glog.Infof("Creating K8S pod: %s . Successfully allocated IP address: %s on custom Subnet: %s", pod.ObjectMeta.Name, cifaddr.String(), csubnet.Subnet.Name)

	// Create Nuage ContainerInterface with given address and Nuage Container
	containerif := new(vspk.ContainerInterface)
	//
	containerif.MAC = vsdclient.GenerateMAC() // XXX - Do we allow / need custom MAC addresses ?
	// containerif.Name = cName                  // XXX - This works only for single container interfaces
	containerif.IPAddress = cifaddr.String()
	containerif.Netmask = csubnet.Subnet.Netmask
	containerif.AttachedNetworkID = csubnet.Subnet.ID
	//
	container := new(vsdclient.Container)
	container.Name = cName
	container.UUID = cUUID
	container.OrchestrationID = k8sOrchestrationID
	// XXX --vspk bug for vspk.Container: "vspk.Container.Intefaces" has to be "[]interface{}
	container.Interfaces = []interface{}{containerif}

	if err := container.Create(); err != nil {
		//
		// State cleanup - release the address, keep the subnet
		csubnet.Range.Release(*cifaddr)
		return nil, bambou.NewBambouError("Error creating K8S Pod: "+pod.ObjectMeta.Name, err.Error())
	}

	// XXX - For new pods, we do not know the pod node at creation time (empty). If so, just add it to the "Pods" cache of running pods
	if pod.Spec.NodeName != "" { // Previously created pod, already scheduled on a node
		err := cniagent.ContainerPUT(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, (*vspk.Container)(container))
		if err != nil {
			glog.Errorf("Creating K8S pod: %s. Failed to submit VSD container: %s to CNI Agent server on host: %s . Error: %s", pod.ObjectMeta.Name, container.Name, pod.Spec.NodeName, err)
		}
		return nil, err
	}

	// Add pod to the cache of pods we are currently processing
	Pods[cName] = container

	return container, nil

}

// Case 3: "Normal" pod --  Allocate an IP address from a non-custom subnet (subnet from ClusterCIDR address space).
// Allocate a non-custom subnet if none exists previously  / no free IP address are available in any of previously exsting non-custom subnets
func case3create(pod *apiv1.Pod) (*vsdclient.Container, error) {
	// Do _NOT_ change those conventions -- the CNI agent relies on them.
	// Container Name
	cName := pod.ObjectMeta.Name + "_" + pod.ObjectMeta.Namespace
	// Container UUID
	cUUID := strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1) + strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1)
	// XXX - Above we made sure this is not nil (VSD Zone is created)
	podNsZone := Namespaces[pod.ObjectMeta.Namespace]

	// Container interface IP address
	var cifaddr *net.IP
	// Container subnet
	var csubnet *vsdclient.Subnet

	for _, subnet := range podNsZone.Subnets {
		if subnet.Customed {
			continue
		}
		// Try to allocate an IP address from a non-custom subnet, if any exist
		if allocd, err := subnet.Range.AllocateNext(); err != nil { // Cannot allocate an IP address on this subnet
			continue
		} else {
			// Successfully allocated an interface on an existing non-custom subnet. Save it.
			cifaddr = &allocd
			csubnet = &subnet
			break
		}
	}

	if cifaddr == nil { // We could not get any lease from any non-custom subnet above
		var newsubnet vsdclient.Subnet
		// Allocate a new cidr from the set of "FreeCIDRs"
		for newprefix, newcidr := range vsdclient.FreeCIDRs {
			if newcidr != nil { // True for any valid entry in the map
				// Grab this prefix and build a vsdclient.Subnet for it.
				newsubnet = vsdclient.Subnet{
					Subnet: &vspk.Subnet{
						Name:    fmt.Sprintf("%s-%d", pod.ObjectMeta.Namespace, len(podNsZone.Subnets)),
						Address: newprefix,
						Netmask: fmt.Sprintf("%d.%d.%d.%d", newcidr.Mask[0], newcidr.Mask[1], newcidr.Mask[2], newcidr.Mask[3]),
					},
					Range:    ipallocator.NewCIDRRange(newcidr),
					Customed: false,
				}

				// Try to alocate an IP address on this subnet Range
				if allocd, err := newsubnet.Range.AllocateNext(); err != nil {
					continue
				} else {
					// Add the subnet to the VSD
					if err := podNsZone.Zone.AddSubnet(newsubnet); err != nil {
						// Release the IP address from the range
						newsubnet.Range.Release(allocd)
						continue
					}
					cifaddr = &allocd
				}

				// Save this as pod's subnet
				csubnet = &newsubnet

				// Append it to this of Subnets for pod's namespace
				podNsZone.Subnets = append(podNsZone.Subnets, newsubnet)

				// Update the Namespace information
				Namespaces[pod.ObjectMeta.Namespace] = podNsZone

				break
			}
		}
	}

	glog.Infof("Creating K8S pod: %s . Successfully allocated IP address: %s on Subnet: %s", pod.ObjectMeta.Name, cifaddr.String(), csubnet.Subnet.Name)

	// Create Nuage ContainerInterface with given address and Nuage Container
	containerif := new(vspk.ContainerInterface)

	//
	containerif.MAC = vsdclient.GenerateMAC()
	containerif.IPAddress = cifaddr.String()
	// containerif.Name = cName // XXX - This works only for single container interfaces
	containerif.Netmask = csubnet.Subnet.Netmask
	containerif.AttachedNetworkID = csubnet.Subnet.ID
	//
	container := new(vsdclient.Container)
	container.Name = cName
	container.UUID = cUUID
	container.OrchestrationID = k8sOrchestrationID
	// XXX --vspk bug for vspk.Container: "vspk.Container.Intefaces" has to be "[]interface{}
	container.Interfaces = []interface{}{containerif}

	if err := container.Create(); err != nil {
		//
		// State cleanup - release the address, keep the subnet
		csubnet.Range.Release(*cifaddr)
		return nil, bambou.NewBambouError("Error creating K8S Pod: "+pod.ObjectMeta.Name, err.Error())
	}

	// XXX - For new pods, we do not know the pod node at creation time (empty). If so, just add it to the "Pods" cache of running pods

	if pod.Spec.NodeName != "" { // Previously created pod, already scheduled on a node
		err := cniagent.ContainerPUT(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, (*vspk.Container)(container))
		if err != nil {
			glog.Errorf("Creating K8S pod: %s. Failed to submit VSD container: %s to CNI Agent server on host: %s . Error: %s", pod.ObjectMeta.Name, container.Name, pod.Spec.NodeName, err)
		}
		return nil, err
	}

	// Add pod to the cache of pods we are currently processing
	Pods[cName] = container

	return container, nil
}
