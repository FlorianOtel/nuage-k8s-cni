package vsd

import (
	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"

	"github.com/nuagenetworks/vspk-go/vspk"
)

////
////  Wrapper around vspk constructs

// Subnet -- wrapper around vspk.Subnet, with custom IPAM
type Subnet struct {
	*vspk.Subnet                    // VSD Subnet. 1-1 mapping (transparent)
	Range        *ipallocator.Range // IPAM for this Subnet
	Customed     bool               // If the net is part of ClusterCIDR or a customed network
}

type NetworkMacro vspk.EnterpriseNetwork

type NetworkMacroGroup vspk.NetworkMacroGroup

type Zone vspk.Zone

type Container vspk.Container
