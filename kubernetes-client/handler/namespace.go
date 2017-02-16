package handler

import (
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	// "github.com/FlorianOtel/client-go/pkg/util/wait"
)

func NamespaceCreated(namespace *apiv1.Namespace) error {
	glog.Info("=====> A namespace got created")
	JsonPrettyPrint("namespace", namespace)
	return nil
}

func NamespaceDeleted(namespace *apiv1.Namespace) error {
	glog.Info("=====> A namespace got deleted")
	JsonPrettyPrint("namespace", namespace)
	return nil
}

// Still TBD if / when / how to use  -- stub so far
func NamespaceUpdated(old, updated *apiv1.Namespace) error {
	return nil
}
