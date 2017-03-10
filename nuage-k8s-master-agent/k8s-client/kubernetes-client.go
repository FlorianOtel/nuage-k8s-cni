package k8s

import (
	"github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/config"
	vsd "github.com/OpenPlatformSDN/nuage-k8s-cni/nuage-k8s-master-agent/vsd-client"

	"net/http"

	"github.com/golang/glog"

	"github.com/FlorianOtel/client-go/kubernetes"
	"github.com/FlorianOtel/client-go/pkg/util/wait"
	"github.com/FlorianOtel/client-go/tools/clientcmd"
	"github.com/FlorianOtel/go-bambou/bambou"
	"github.com/nuagenetworks/vspk-go/vspk"
)

////////
//////// Mappings of K8S constructs to VSD constructs
////////

//// XXX - Notes:
//// - Since there is a single Client active at one time, they can become inaccurate only if the object is manipulated (changed/deleted) directly in the VSD.
//// - The data structs below contain the K8S view of things / populated in response to K8S events. Must be reconciled with the data in the VSD.

// A K8S namespace
type namespace struct {
	*vspk.Zone              // The VSD Zone. 1-1 mapping (transparent)
	Subnets    []vsd.Subnet // List of subnets associated with this namespace
}

// K8S Pod -- Wrapper around vspk.Container
// ...
// ....

// XXX -- For now we allow a pod to be attached to a single Subnet. To be addressed later.
type pod struct {
	*vspk.Container
	vsd.Subnet
}

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
	// Pods map[string]pod // Key: Pod Name

	////
	//// Services
	////
	// Services map[string]*vspk.EnterpriseNetwork // Key: Service Name
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
	////
	////
	////

	/*

		////
		//// Initialize local state
		////
		Namespaces = make(map[string]namespace)
		Pods = make(map[string]pod)
		Services = make(map[string]*vspk.EnterpriseNetwork)
		////
		////
		////

	*/

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
