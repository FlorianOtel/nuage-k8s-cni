package types

import (
	cnitypes "github.com/containernetworking/cni/pkg/types"
	currentcni "github.com/containernetworking/cni/pkg/types/current"
)

const (
	NuageCNIPlugin = "nuage"
)

// Wrapper around CNI NetConf (current: v0.3.0)
type NetConf struct {
	cnitypes.NetConf        // XXX -- Includes CNIVersion
	ID               string `json:"id"`
	Prefix           string `json:"prefix"`
	Gateway          string `json:"gateway"`
}

// Our wrapper around CNI Result (current: v0.3.0)
type Result struct {
	CNIVersion string `json:"cniVersion,omitempty"` // XXX - (??) Currently (Mar 2017) absent from 'currentcni.Result'
	currentcni.Result
}

/*
// Our wrapper around CNI Interface (current: v0.3.0)
type Interface struct {
	currentcni.Interface
	NetwkName string `json:"network-name"`
}
*/

//// CNI Result wrapper methods (as per v0.3.0)
func (r *Result) Version() string {
	return r.Result.Version()
}

/*
// FIX ME
func (r *Result) GetAsVersion(version string) (cnitypes.Result , error) {
	newres, err := r.Result.GetAsVersion(version)
	r.Result = newres
	return r, err
}
*/

func (r *Result) Print() error {
	return r.Result.Print()
}

func (r *Result) String() string {
	return r.Result.String()
}
