package handler

import (
	"github.com/golang/glog"
	//

	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	// "github.com/FlorianOtel/client-go/pkg/util/wait"
)

func PodCreated(pod *apiv1.Pod) error {
	glog.Info("=====> A pod got created")
	JsonPrettyPrint("pod", pod)
	return nil
}

func PodDeleted(pod *apiv1.Pod) error {
	glog.Info("=====> A pod got deleted")
	JsonPrettyPrint("pod", pod)
	return nil
}

// Still TBD if / when / how to use  -- stub so far
func PodUpdated(old, updated *apiv1.Pod) error {
	return nil
}
