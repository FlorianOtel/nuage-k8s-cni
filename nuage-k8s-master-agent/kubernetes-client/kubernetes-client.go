package k8s

import (
	// "github.com/OpenPlatformSDN/nuage-cni-k8s/nuage-k8s-master-agent/agent"

	"github.com/OpenPlatformSDN/nuage-cni-k8s/nuage-k8s-master-agent/config"
	"k8s.io/kubernetes/pkg/registry/core/service/ipallocator"

	"net/http"

	"github.com/golang/glog"

	"github.com/FlorianOtel/client-go/kubernetes"
	"github.com/FlorianOtel/client-go/pkg/util/wait"
	"github.com/FlorianOtel/client-go/tools/clientcmd"
	"github.com/FlorianOtel/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
	//
	// apiv1 "k8s.io/kubernetes/pkg/api/v1"
	// "k8s.io/kubernetes/pkg/apis/extensions"
	// k8sfields "k8s.io/kubernetes/pkg/fields"
	// k8slabels "k8s.io/kubernetes/pkg/labels"
)

////////
//////// Mappings of K8S constructs to VSD constructs
////////

//// XXX - Notes:
//// - Since there is a single Client active at one time, they can be innacurate only if the object is directly deleted in the VSD.
//// - The data structs below contain the K8S view of things / populated in response to K8S events. Must be reconciled with VSD data

// A K8S namespace is a set (map) of one or more subnets. Key is the Subnet/Range index
type namespace struct {
	*vspk.Zone          // The VSD Zone. 1-1 mapping (transparent)
	Subnets    []subnet // List of subnets associated with this namespace
}

// K8S Pod -- Wrapper around vspk.Container
// XXX -- Following the VSD concept, we "extend" the Pod concept to have multiple ifaces / attached to multiple subnets
type pod struct {
	*vspk.Container
	Subnets []subnet // List of subnets this pod is attached to
}

// K8S Subnet --  Wrapper around vspk.Subnet
type subnet struct {
	*vspk.Subnet                    // VSD Subnet. 1-1 mapping (transparent)
	Range        *ipallocator.Range // The Range of this Subnet
}

////////
////////
////////

var (
	clientset      *kubernetes.Clientset
	UseNetPolicies = false

	////
	//// K8S namespaces
	////
	Namespaces map[string]namespace // Key: Namespace Name
	// Pre-defined namespaces
	PrivilegedNS = "kube-system"
	DefaultNS    = "default"

	////
	//// Pods
	////
	Pods map[string]pod // Key: Pod Name

	////
	//// Services
	////
	Services map[string]*vspk.EnterpriseNetwork // Key: Service Name
)

func InitClient(conf *config.AgentConfig) error {

	kubeconfig := &conf.KubeConfigFile

	// uses the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		return bambou.NewBambouError("Error parsing kubeconfig", err.Error())
	}

	glog.Infof("Loaded Agent kubeconfig: %s ", *kubeconfig)

	clientset, err = kubernetes.NewForConfig(config)
	if err != nil {
		return bambou.NewBambouError("Error creating Kubernetes client", err.Error())
	}

	////////
	//////// Discover K8S API -- version, extensions: Check if server supports Network Policy API extension (currently / Dec 2016: apiv1beta1)
	////////

	sver, _ := clientset.ServerVersion()

	glog.Infof("Successfully logged in Kuberentes server. Server details: %#v", *sver)

	sres, _ := clientset.ServerResources()

	for _, res := range sres {
		for _, apires := range res.APIResources {
			switch apires.Name {
			case "networkpolicies":
				glog.Infof("Found Kubernetes API server support for %#v. Available under / GroupVersion is: %#v . APIResource details: %#v", apires.Name, res.GroupVersion, apires)
				UseNetPolicies = true
			default:
				// glog.Infof("Kubernetes API Server discovery: API Server Resource:\n%#v\n", apires)
			}
		}
	}
	////
	//// Initialize local state
	////
	Namespaces = make(map[string]namespace)
	Pods = make(map[string]pod)
	Services = make(map[string]*vspk.EnterpriseNetwork)
	////
	////
	////
	glog.Info("Kubernetes client initialization completed")
	return nil
}

func EventWatcher() {
	////////
	//////// Watch Pods
	////////

	//Create a cache to store Pods
	// var store cache.Store
	// store, pController := CreatePodController(clientset, "default", PodCreated, PodDeleted, PodUpdated)

	_, pController := CreatePodController(clientset, "", "default", PodCreated, PodDeleted, PodUpdated)
	go pController.Run(wait.NeverStop)

	////////
	//////// Watch Services
	////////

	_, sController := CreateServiceController(clientset, "default", ServiceCreated, ServiceDeleted, ServiceUpdated)
	go sController.Run(wait.NeverStop)

	////////
	//////// Watch Namespaces
	////////

	_, nsController := CreateNamespaceController(clientset, NamespaceCreated, NamespaceDeleted, NamespaceUpdated)
	go nsController.Run(wait.NeverStop)

	////////
	//////// Watch NetworkPolicies (if supported)
	////////

	if UseNetPolicies {

		_, npController := CreateNetworkPolicyController(clientset, "default", NetworkPolicyCreated, NetworkPolicyDeleted, NetworkPolicyUpdated)
		go npController.Run(wait.NeverStop)

	}
	//Keep alive
	glog.Error(http.ListenAndServe(":8099", nil))
}
