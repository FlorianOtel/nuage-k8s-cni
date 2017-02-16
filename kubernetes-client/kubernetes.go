package k8s

import (
	"github.com/OpenPlatformSDN/nuage-cni-k8s/config"

	"net/http"

	"github.com/golang/glog"

	"github.com/FlorianOtel/client-go/kubernetes"
	"github.com/FlorianOtel/client-go/pkg/util/wait"
	"github.com/FlorianOtel/client-go/tools/clientcmd"
	"github.com/OpenPlatformSDN/nuage-cni-k8s/kubernetes-client/handler"
	//
	// apiv1 "k8s.io/kubernetes/pkg/api/v1"
	// "k8s.io/kubernetes/pkg/apis/extensions"
	// k8sfields "k8s.io/kubernetes/pkg/fields"
	// k8slabels "k8s.io/kubernetes/pkg/labels"
)

var (
	UseNetPolicies = false
	// Pre-defined namespaces
	PrivilegedNS = "kube-system"
	DefaultNS    = "default"
)

func Client(conf *config.AgentConfig) {

	kubeconfig := &conf.KubeConfigFile

	// uses the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		glog.Fatalf("Error parsing kubeconfig. Error: %s", err)
	}

	glog.Infof("Loaded Agent kubeconfig: %s ", *kubeconfig)

	// creates the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Errorf("Error creating Kubernetes client. Error: %s", err)
	}

	////////
	//////// Discover K8S API -- version, extensions: Check if server supports Network Policy API extension (currently / Dec 2016: apiv1beta1)
	////////

	sver, err := clientset.ServerVersion()

	glog.Infof("Successfully logged in Kuberentes server. Server details: %#v", *sver)
	//
	sres, err := clientset.ServerResources()

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

	////////
	//////// Watch Pods
	////////

	//Create a cache to store Pods
	// var store cache.Store
	// store, pController := handler.CreatePodController(clientset, "default", handler.PodCreated, handler.PodDeleted, handler.PodUpdated)

	_, pController := handler.CreatePodController(clientset, "", "default", handler.PodCreated, handler.PodDeleted, handler.PodUpdated)
	go pController.Run(wait.NeverStop)

	////////
	//////// Watch Services
	////////

	_, sController := handler.CreateServiceController(clientset, "default", handler.ServiceCreated, handler.ServiceDeleted, handler.ServiceUpdated)
	go sController.Run(wait.NeverStop)

	////////
	//////// Watch Namespaces
	////////

	_, nsController := handler.CreateNamespaceController(clientset, handler.NamespaceCreated, handler.NamespaceDeleted, handler.NamespaceUpdated)
	go nsController.Run(wait.NeverStop)

	////////
	//////// Watch NetworkPolicies (if supported)
	////////

	if UseNetPolicies {

		_, npController := handler.CreateNetworkPolicyController(clientset, "default", handler.NetworkPolicyCreated, handler.NetworkPolicyDeleted, handler.NetworkPolicyUpdated)
		go npController.Run(wait.NeverStop)

	}
	//Keep alive
	glog.Error(http.ListenAndServe(":8099", nil))
}
