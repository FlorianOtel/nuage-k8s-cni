package handler

import (
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	// "github.com/FlorianOtel/client-go/pkg/util/wait"
)

func ServiceCreated(service *apiv1.Service) error {
	glog.Info("=====> A service got created")
	JsonPrettyPrint("service", service)
	return nil
}

func ServiceDeleted(service *apiv1.Service) error {
	glog.Info("=====> A service got deleted")
	JsonPrettyPrint("service", service)
	return nil
}

// Still TBD if / when / how to use  -- stub so far
func ServiceUpdated(old, updated *apiv1.Service) error {
	return nil
}
