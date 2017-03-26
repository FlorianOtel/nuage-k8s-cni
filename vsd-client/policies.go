package vsd

import (
	"github.com/golang/glog"

	netpolicy "github.com/OpenPlatformSDN/nuage-policy-framework"
)

const (
	// Policy names for Egress / Ingress
	epname = "Egress Policy for K8S"
	ipname = "Ingress Policy for K8S"
)

var (
	// Private, low priority "allow all" traffic for network egress
	egressPolicy *netpolicy.Policy
	// Actual policy rules are imposed on network ingress
	IngressPolicy *netpolicy.Policy
)

// Initialize network policies for the Domain:
// - Egress: low priority, single "allow all traffic" PolicyElement
// - Ingress: At this stage, a single entry allowing intra-namespace traffic

func initPolicies() error {

	policies, perr := (*netpolicy.PolicyDomain)(Domain).GetPolicies()

	if perr != nil {
		return perr
	}

	for _, policy := range policies {
		if epname == policy.Name && policy.Type == netpolicy.Egress {
			egressPolicy = policy
			glog.Infof("The domain already has an existing %s: %s", epname, egressPolicy)
			break
		}
	}

	if egressPolicy == nil { // Not found above
		egressPolicy, _ := netpolicy.NewPolicy(epname, netpolicy.Egress, Enterprise.Name, Domain.Name, 999999999)
		// Create a Policy Element allowing all egress traffic
		aaegressPE := netpolicy.AllowAllEgressPE
		egressPolicy.AttachPE(&aaegressPE)
		if err := (*netpolicy.PolicyDomain)(Domain).ApplyPolicy(egressPolicy); err != nil {
			return err
		}
		glog.Infof("Successfully applied Egress Policy: %s", *egressPolicy)
	}

	// Ingress -- Basic is a lowest priority "allow traffic to endpoint Zone" <--> allow traffic btw. pods in the same namespace

	for _, policy := range policies {
		if ipname == policy.Name && policy.Type == netpolicy.Ingress {
			IngressPolicy = policy
			glog.Infof("The domain already has an existing %s: %s", ipname, IngressPolicy)
			break
		}
	}

	if IngressPolicy == nil { // Not found above
		IngressPolicy, _ = netpolicy.NewPolicy(ipname, netpolicy.Ingress, Enterprise.Name, Domain.Name, 999999999)
		// Create a PolicyElement allowing ingress traffic to endpoint's own Zone
		aaizone := netpolicy.PolicyElement{
			Name:        "Allow intra-namespace traffic",
			Priority:    999999999,
			From:        netpolicy.AllSrcsIngress,
			To:          netpolicy.PolicyDstScope{Type: string(netpolicy.MyZone)},
			TrafficSpec: netpolicy.MatchAllTraffic,
			Action:      netpolicy.Allow,
		}
		IngressPolicy.AttachPE(&aaizone)
		if err := (*netpolicy.PolicyDomain)(Domain).ApplyPolicy(IngressPolicy); err != nil {
			return err
		}
		glog.Infof("Successfully applied Ingress Policy: %s", *IngressPolicy)

	}

	return nil
}
