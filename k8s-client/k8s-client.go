package k8s

import (
	"github.com/OpenPlatformSDN/nuage-k8s-cni/config"
	vsdclient "github.com/OpenPlatformSDN/nuage-k8s-cni/vsd-client"

	"net/http"

	"github.com/golang/glog"

	"github.com/FlorianOtel/go-bambou/bambou"
	"github.com/OpenPlatformSDN/client-go/kubernetes"
	"github.com/OpenPlatformSDN/client-go/pkg/util/wait"
	"github.com/OpenPlatformSDN/client-go/tools/clientcmd"
)

////////
//////// Mappings of K8S constructs to VSD constructs
////////

//// XXX - Notes:
//// - Since there is a single Client active at one time, they can become inaccurate only if the object is manipulated (changed/deleted) directly in the VSD.
//// - The data structs below contain the K8S view of things / populated in response to K8S events. Must be reconciled with the data in the VSD.

// A K8S namespace
type namespace struct {
	*vsdclient.Zone                    // The VSD Zone. 1-1 mapping (transparent)
	Subnets         []vsdclient.Subnet // List of subnets associated with this namespace
}

// The Orchestration ID used by the CNI Plugin to identify the platform (slightly different clients for different platforms)
const (
	k8sOrchestrationID = "Kubernetes"
)

var (
	clientset      *kubernetes.Clientset
	UseNetPolicies = false

	////
	//// K8S namespaces
	////
	Namespaces map[string]namespace // Key: K8S Namespace Name

	// Pre-defined namespaces
	PrivilegedNS = "kube-system"
	DefaultNS    = "default"

	////
	//// Pod cache -- pods currently processing
	////

	Pods map[string]*vsdclient.Container // Key: vspk.Container.Name == pod.ObjectMeta.Name + "_" + pod.ObjectMeta.Namespace

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
	Pods = make(map[string]*vsdclient.Container)
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

	_, pController := CreatePodController(clientset, "", "", PodCreated, PodDeleted, PodUpdated)
	go pController.Run(wait.NeverStop)

	////////
	//////// Watch Services
	////////

	_, sController := CreateServiceController(clientset, "", ServiceCreated, ServiceDeleted, ServiceUpdated)
	go sController.Run(wait.NeverStop)

	////////
	//////// Watch Namespaces
	////////

	_, nsController := CreateNamespaceController(clientset, "", NamespaceCreated, NamespaceDeleted, NamespaceUpdated)
	go nsController.Run(wait.NeverStop)

	////////
	//////// Watch NetworkPolicies (if supported)
	////////

	if UseNetPolicies {

		_, npController := CreateNetworkPolicyController(clientset, "", NetworkPolicyCreated, NetworkPolicyDeleted, NetworkPolicyUpdated)
		go npController.Run(wait.NeverStop)

	}
	//Keep alive
	glog.Error(http.ListenAndServe(":8099", nil))
}
