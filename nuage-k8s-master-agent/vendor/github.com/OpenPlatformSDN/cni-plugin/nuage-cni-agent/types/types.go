package agenttypes

////
//// Common types and constants for CNI Agent server and client library
////

import "time"

const (
	MAX_CONNS = 256             // How many simultaneous (pending) connections to the CNI agent server can we have
	MAX_IDLE  = 7 * time.Second // How long we should wait for CNI Agent server
	// Relative paths for the agent server
	NetconfPath   = "/cni/networks/"     // Agent server relative path for CNI NetConf
	ResultPath    = "/cni/interfaces/"   // Agent server relative path for CNI Result
	ContainerPath = "/nuage/containers/" // Agent server relative path for vspk.Container cache
)
