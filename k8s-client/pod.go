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

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
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

	var podNsZone namespace // XXX -- The name is the VSD Zone name (different from namespace name itself)
	var exists, found bool

	if podNsZone, exists = Namespaces[pod.ObjectMeta.Namespace]; !exists {
		// Wait for a max 10 seconds, probing local cache
		timeChan := time.NewTimer(time.Second * 10).C
		tickChan := time.NewTicker(time.Millisecond * 100).C

		for {
			select {
			case <-timeChan:
				return bambou.NewBambouError("Error creating K8S Pod: "+pod.ObjectMeta.Name, "Timeout waiting for namespace "+pod.ObjectMeta.Namespace+" to be created")
			case <-tickChan:
				if podNsZone, found = Namespaces[pod.ObjectMeta.Namespace]; !found {
					continue
				}
			}
			break
		}
	}

	// XXX -- at this point "podNsZone" points to a valid "namespace"

	// Do _NOT_ change those conventions -- the CNI agent relies on them.
	// Container Name
	cName := pod.ObjectMeta.Name + "_" + pod.ObjectMeta.Namespace
	// Container UUID
	cUUID := strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1) + strings.Replace(string(pod.ObjectMeta.UID), "-", "", -1)

	/////
	///// Get pod networking details.
	/////

	// Case 1: Pod already has a VSD container associated with it (at startup, previously existing pod).

	if container, err := vsdclient.ExistsContainer(cName); err != nil {
		return bambou.NewBambouError("Error creating K8S Pod: "+pod.ObjectMeta.Name, err.Error())
	} else {
		if container != nil {
			cIPv4Addr, cIPv4Mask := vsdclient.ContainerIPandMask(container)
			// XXX - No need to handle IPAM here. The container interface was allocated when we parsed the corresponding Subnet
			cifaddr := net.IPNet{net.ParseIP(cIPv4Addr).To4(), net.IPMask(net.ParseIP(cIPv4Mask).To4())}
			glog.Infof("Creating K8S pod: %s already created. VSD container details: Name: %s . UUID: %s . IP address: %s", pod.ObjectMeta.Name, container.Name, cUUID, cifaddr.String())

			// XXX - At startup previously existing pods have a valid "pod.Spec.NodeName", so this error checking is a bit overkill
			if err := cniagent.ContainerPUT(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, container); err == nil {
				glog.Infof("Creating K8S pod: %s . Successfully submitted VSD container: %s to CNI Agent server on host: %s", pod.ObjectMeta.Name, container.Name, pod.Spec.NodeName)
			}

			return nil
		}
	}

	// Case 2: "Normal" pod --  Allocate an IP address from a non-custom subnet (subnet from ClusterCIDR address space).
	// Allocate a non-custom subnet if none exists previously  / no free IP address are available in any of those subnets

	// Container interface MAC address
	cmac := vsdclient.GenerateMAC()
	// Container interface IP address
	var cifaddr *net.IP
	// Container subnet
	csubnet := vsdclient.Subnet{}

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
			csubnet = subnet
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
					if err := vsdclient.ZoneAddSubnet(podNsZone.Zone, newsubnet); err != nil {
						// Release the IP address from the range
						newsubnet.Range.Release(allocd)
						continue
					}
					cifaddr = &allocd
				}

				// Save this as pod's subnet
				csubnet = newsubnet

				// Append it to this of Subnets for pod's namespace
				podNsZone.Subnets = append(podNsZone.Subnets, csubnet)

				// Update the Namespace information
				Namespaces[pod.ObjectMeta.Namespace] = podNsZone

				break
			}
		}
	}

	///

	glog.Infof("Creating K8S pod: %s . Successfully allocated IP address: %s on Subnet: %s", pod.ObjectMeta.Name, cifaddr.String(), csubnet.Subnet.Name)

	// Case 3: "Custom" pod --  Custom network settings:  MAC / IP address / Custom subnet...
	//

	/*
	 Still TBD
	 - Authentication
	 - What pod networking details we allow customizing: IP address / Subnet / MAC / gateway / FloatingIP....
	 - What components need to be created in the VSD vs created automatically ?
	 - ....
	 - ....
	*/

	// Create Nuage ContainerInterface with given address and Nuage Container

	containerif := new(vspk.ContainerInterface)
	//
	// XXX -- Still TBD -- do we allow container interfaces created with custom MACs ?
	// ...
	// ...
	containerif.MAC = cmac
	containerif.IPAddress = cifaddr.String()
	containerif.Netmask = csubnet.Subnet.Netmask
	containerif.AttachedNetworkID = csubnet.Subnet.ID

	container := new(vspk.Container)
	container.Name = cName
	container.UUID = cUUID
	container.OrchestrationID = k8sOrchestrationID
	// XXX --vspk bug for vspk.Container: "vspk.Container.Intefaces" has to be "[]interface{}
	container.Interfaces = []interface{}{containerif}

	if err := vsdclient.CreateContainer(container); err != nil {
		//
		// State cleanup - release the address, keep the subnet
		csubnet.Range.Release(*cifaddr)
		return bambou.NewBambouError("Error creating K8S Pod: "+pod.ObjectMeta.Name, err.Error())
	}

	// XXX - For new pods, we do not know the pod node at creation time (empty). If so, just add it to the "Pods" cache of running pods

	if pod.Spec.NodeName != "" { // Previously created pod, already scheduled on a node
		err := cniagent.ContainerPUT(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, container)
		if err != nil {
			glog.Errorf("Creating K8S pod: %s. Failed to submit VSD container: %s to CNI Agent server on host: %s . Error: %s", pod.ObjectMeta.Name, container.Name, pod.Spec.NodeName, err)
		}
		return err
	}

	// Add pod to the cache of pods we are currently processing
	Pods[cName] = container
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

	container, err := cniagent.ContainerGET(cniclient.AgentClient, pod.Spec.NodeName, cniclient.AgentServerPort, cName)
	if err != nil {
		glog.Errorf("Deleting K8S Pod: %s . Cannot fecth container: %s from CNI Agent server on host: %s. Error: %s", pod.ObjectMeta.Name, cName, pod.Spec.NodeName, err.Error())
		return err
	}

	// Get the IP address of the container
	cIPv4Addr, cIPv4Mask := vsdclient.ContainerIPandMask(container)
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
			if err := cniagent.ContainerPUT(cniclient.AgentClient, updated.Spec.NodeName, cniclient.AgentServerPort, container); err != nil {
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
