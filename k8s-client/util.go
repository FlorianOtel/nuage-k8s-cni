package k8s

import (
	"encoding/json"
	"fmt"

	"github.com/golang/glog"
	//

	apiv1 "github.com/OpenPlatformSDN/client-go/pkg/api/v1"
	apiv1beta1 "github.com/OpenPlatformSDN/client-go/pkg/apis/extensions/v1beta1"
	"github.com/OpenPlatformSDN/client-go/pkg/runtime"
	// "github.com/OpenPlatformSDN/client-go/pkg/util/wait"
)

// Pretty Prints (JSON) for a Kubernetes API object:
// - The "ObjectMeta"  is common to all the API objects and is handled identically, disregarding of the underlying type
// - The "Spec" is specific to each reasource and is handled on per-object specific basis (even if the field -- "Spec" -- is named the same for all objects)

func JsonPrettyPrint(resource string, obj runtime.Object) error {
	var meta apiv1.ObjectMeta
	var jsonmeta, jsonspec []byte
	var err error

	switch resource {
	case "pod":
		meta = obj.(*apiv1.Pod).ObjectMeta
		jsonspec, err = json.MarshalIndent(obj.(*apiv1.Pod).Spec, "", " ")
	case "service":
		meta = obj.(*apiv1.Service).ObjectMeta
		jsonspec, err = json.MarshalIndent(obj.(*apiv1.Service).Spec, "", " ")
	case "namespace":
		meta = obj.(*apiv1.Namespace).ObjectMeta
		jsonspec, err = json.MarshalIndent(obj.(*apiv1.Namespace).Spec, "", " ")
	case "networkpolicy":
		meta = obj.(*apiv1beta1.NetworkPolicy).ObjectMeta
		jsonspec, err = json.MarshalIndent(obj.(*apiv1beta1.NetworkPolicy).Spec, "", " ")
	default:
		glog.Errorf("Don't know how to pretty-print API object: %s", resource)
	}

	// If any of the JSON marshalling of the object-specific specs above returned an error
	if err != nil {
		return err
	}

	// JSON pretty-print the ObjectMeta -- unlike "Spec" it's the same for all type of objects (has it's own type)
	jsonmeta, err = json.MarshalIndent(meta, "", " ")

	if err != nil {
		return err
	}

	fmt.Printf("====> %s <====\n ######## %s ObjectMetadata ########\n%s\n ######## %s Spec ########\n%s\n\n ", resource, resource, string(jsonmeta), resource, string(jsonspec))

	return err
}
