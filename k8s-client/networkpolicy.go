package k8s

import (
	apiv1beta1 "github.com/OpenPlatformSDN/client-go/pkg/apis/extensions/v1beta1"
	"github.com/golang/glog"
)

// "github.com/OpenPlatformSDN/client-go/pkg/util/wait"

func NetworkPolicyCreated(networkpolicy *apiv1beta1.NetworkPolicy) error {
	glog.Info("=====> A networkpolicy got created")
	JsonPrettyPrint("networkpolicy", networkpolicy)
	return nil
}

func NetworkPolicyDeleted(networkpolicy *apiv1beta1.NetworkPolicy) error {
	glog.Info("=====> A networkpolicy got deleted")
	JsonPrettyPrint("networkpolicy", networkpolicy)
	return nil
}

// Still TBD if / when / how to use  -- stub so far
func NetworkPolicyUpdated(old, updated *apiv1beta1.NetworkPolicy) error {
	return nil
}
