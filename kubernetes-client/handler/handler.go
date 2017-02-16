/*

Attribution for this code: Our dearest friends at Aporeto -- see https://www.aporeto.com/trireme/.
Original code: https://github.com/aporeto-inc/trireme-kubernetes/blob/master/kubernetes/handler.go

*/

package handler

import (
	"time"

	"github.com/golang/glog"
	//
	"github.com/FlorianOtel/client-go/kubernetes"
	apiv1 "github.com/FlorianOtel/client-go/pkg/api/v1"
	apiv1beta1 "github.com/FlorianOtel/client-go/pkg/apis/extensions/v1beta1"
	"github.com/FlorianOtel/client-go/pkg/fields"
	"github.com/FlorianOtel/client-go/pkg/runtime"
	"github.com/FlorianOtel/client-go/tools/cache"
	// "github.com/FlorianOtel/client-go/pkg/util/wait"
)

// CreateResourceController creates a controller for a specific ressource and namespace.
// The parameter function will be called on Add/Delete/Update events
func CreateResourceController(client cache.Getter, resource string, namespace string, obj runtime.Object, selector fields.Selector,
	addFunc func(addedObj interface{}), deleteFunc func(deletedObj interface{}), updateFunc func(oldObj, updatedObj interface{})) (cache.Store, *cache.Controller) {

	handlers := cache.ResourceEventHandlerFuncs{
		AddFunc:    addFunc,
		DeleteFunc: deleteFunc,
		UpdateFunc: updateFunc,
	}

	listWatch := cache.NewListWatchFromClient(client, resource, namespace, selector)
	store, controller := cache.NewInformer(listWatch, obj, time.Millisecond*0, handlers)
	return store, controller
}

// CreatePodsController creates a controller specifically for Pods.
// XXX - If a hostname is specified, then we limit the controller to that particular node. It's the caller responsibility to validate the node name (no checks performed here)
func CreatePodController(c *kubernetes.Clientset, hostname string, namespace string,
	addFunc func(addedObj *apiv1.Pod) error, deleteFunc func(deletedObj *apiv1.Pod) error, updateFunc func(oldObj, updatedObj *apiv1.Pod) error) (cache.Store, *cache.Controller) {

	var filter fields.Selector

	if hostname == "" {
		filter = fields.Everything()
	} else {
		filter = fields.Set(map[string]string{
			"spec.nodeName": hostname,
		}).AsSelector()
	}

	return CreateResourceController(c.Core().RESTClient(), "pods", namespace, &apiv1.Pod{}, filter,
		func(addedObj interface{}) {
			if err := addFunc(addedObj.(*apiv1.Pod)); err != nil {
				glog.Infof("Error while handling Add Pod: %s ", err)
			}
		},
		func(deletedObj interface{}) {
			if err := deleteFunc(deletedObj.(*apiv1.Pod)); err != nil {
				glog.Infof("Error while handling Delete Pod: %s ", err)
			}
		},
		func(oldObj, updatedObj interface{}) {
			if err := updateFunc(oldObj.(*apiv1.Pod), updatedObj.(*apiv1.Pod)); err != nil {
				glog.Infof("Error while handling Update Pod: %s ", err)
			}
		})
}

// CreateServiceController creates a controller specifically for Services.
func CreateServiceController(c *kubernetes.Clientset, namespace string,
	addFunc func(addedObj *apiv1.Service) error, deleteFunc func(deletedObj *apiv1.Service) error, updateFunc func(oldObj, updatedObj *apiv1.Service) error) (cache.Store, *cache.Controller) {
	return CreateResourceController(c.Core().RESTClient(), "services", namespace, &apiv1.Service{}, fields.Everything(),
		func(addedObj interface{}) {
			if err := addFunc(addedObj.(*apiv1.Service)); err != nil {
				glog.Infof("Error while handling Add service: %s ", err)
			}
		},
		func(deletedObj interface{}) {
			if err := deleteFunc(deletedObj.(*apiv1.Service)); err != nil {
				glog.Infof("Error while handling Delete service: %s ", err)
			}
		},
		func(oldObj, updatedObj interface{}) {
			if err := updateFunc(oldObj.(*apiv1.Service), updatedObj.(*apiv1.Service)); err != nil {
				glog.Infof("Error while handling Update service: %s ", err)
			}
		})
}

// CreateNetworkPolicysController creates a controller specifically for NetworkPolicies.
func CreateNetworkPolicyController(c *kubernetes.Clientset, namespace string,
	addFunc func(addedObj *apiv1beta1.NetworkPolicy) error, deleteFunc func(deletedObj *apiv1beta1.NetworkPolicy) error, updateFunc func(oldObj, updatedObj *apiv1beta1.NetworkPolicy) error) (cache.Store, *cache.Controller) {
	return CreateResourceController(c.Extensions().RESTClient(), "networkpolicies", namespace, &apiv1beta1.NetworkPolicy{}, fields.Everything(),
		func(addedObj interface{}) {
			if err := addFunc(addedObj.(*apiv1beta1.NetworkPolicy)); err != nil {
				glog.Infof("Error while handling Add NetworkPolicy: %s ", err)
			}
		},
		func(deletedObj interface{}) {
			if err := deleteFunc(deletedObj.(*apiv1beta1.NetworkPolicy)); err != nil {
				glog.Infof("Error while handling Delete NetworkPolicy: %s ", err)
			}
		},
		func(oldObj, updatedObj interface{}) {
			if err := updateFunc(oldObj.(*apiv1beta1.NetworkPolicy), updatedObj.(*apiv1beta1.NetworkPolicy)); err != nil {
				glog.Infof("Error while handling Update NetworkPolicy: %s ", err)
			}
		})
}

// CreateNamespaceController creates a controller specifically for Namespaces.
func CreateNamespaceController(c *kubernetes.Clientset,
	addFunc func(addedObj *apiv1.Namespace) error, deleteFunc func(deletedObj *apiv1.Namespace) error, updateFunc func(oldObj, updatedObj *apiv1.Namespace) error) (cache.Store, *cache.Controller) {

	return CreateResourceController(c.Core().RESTClient(), "namespaces", "", &apiv1.Namespace{}, fields.Everything(),
		func(addedObj interface{}) {
			if err := addFunc(addedObj.(*apiv1.Namespace)); err != nil {
				glog.Infof("Error while handling Add NameSpace: %s ", err)
			}
		},
		func(deletedObj interface{}) {
			if err := deleteFunc(deletedObj.(*apiv1.Namespace)); err != nil {
				glog.Infof("Error while handling Delete NameSpace: %s ", err)

			}
		},
		func(oldObj, updatedObj interface{}) {
			if err := updateFunc(oldObj.(*apiv1.Namespace), updatedObj.(*apiv1.Namespace)); err != nil {
				glog.Infof("Error while handling Update NameSpace: %s ", err)

			}
		})
}

// CreateNodeController creates a controller specifically for Nodes.
func CreateNodeController(c *kubernetes.Clientset,
	addFunc func(addedObj *apiv1.Node) error, deleteFunc func(deletedObj *apiv1.Node) error, updateFunc func(oldObj, updatedObj *apiv1.Node) error) (cache.Store, *cache.Controller) {
	return CreateResourceController(c.Core().RESTClient(), "nodes", "", &apiv1.Node{}, fields.Everything(),
		func(addedObj interface{}) {
			if err := addFunc(addedObj.(*apiv1.Node)); err != nil {
				glog.Infof("Error while handling Add Node: %s ", err)
			}
		},
		func(deletedObj interface{}) {
			if err := deleteFunc(deletedObj.(*apiv1.Node)); err != nil {
				glog.Infof("Error while handling Delete Node: %s ", err)
			}
		},
		func(oldObj, updatedObj interface{}) {
			if err := updateFunc(oldObj.(*apiv1.Node), updatedObj.(*apiv1.Node)); err != nil {
				glog.Infof("Error while handling Update Node: %s ", err)
			}
		})
}
