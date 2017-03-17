package etcdelection

import (
	"context"
	"io/ioutil"
	"os"

	yaml "gopkg.in/yaml.v1"

	"time"

	"github.com/FlorianOtel/go-bambou/bambou"
	etcderr "github.com/coreos/etcd/error"

	"github.com/OpenPlatformSDN/nuage-k8s-cni/config"

	"github.com/coreos/etcd/client"
	"github.com/golang/glog"
)

////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

type Myetcdclient struct { //etcd client + stuff

	// etcd client Config
	Config client.Config

	// Connection to the server
	kapi client.KeysAPI

	// Latest server reply - metadata
	Resp client.Response

	// client props
	// cancel chan struct{}
}

const (
	// the top etcd directory under which all keys will be created
	Topdir = "/nuageK8Sagent"

	// Nr clients in the cluster
	NrClients = 3

	// ServiceTTL - TTL for etcd keys
	ServiceTTL = time.Second * 9
)

var (
	// K8S Master config -- includes network information and etcd client details
	k8sMasterConfig config.MasterConfig
)

////  Load K8S Master configuration file -- check if any EtcdClientInfo is there. If there isn't any server added, just add the one passed by the CLI (or the default one)
func InitClient(conf *config.AgentConfig) error {
	if data, err := ioutil.ReadFile(conf.MasterConfigFile); err != nil {
		return bambou.NewBambouError("Cannot read K8S Master configuration file: "+conf.MasterConfigFile, err.Error())
	} else {
		if err := yaml.Unmarshal(data, &k8sMasterConfig); err != nil {
			return bambou.NewBambouError("Cannot parse K8S Master configuration file: "+conf.MasterConfigFile, err.Error())
		}
	}

	if len(k8sMasterConfig.EtcdClientInfo.EtcdServerUrls) == 0 {
		// If no etcd servers were present in the K8S Master configuration file, use the CLI flag
		k8sMasterConfig.EtcdClientInfo.EtcdServerUrls = append(k8sMasterConfig.EtcdClientInfo.EtcdServerUrls, conf.EtcdServerUrl)
	}

	return nil
}

// Create a key under the given etcd directory. The key has a TTL of "ServiceTTL"
// XXX - The key must _not_ be prexisting (i.e. "client.PrevNoExist")
func (c *Myetcdclient) CreateKey(dir, key, value string) error {

	resp, err := c.kapi.Set(context.Background(), dir+"/"+key, value, &client.SetOptions{
		Dir:       false,
		PrevExist: client.PrevNoExist,
		TTL:       ServiceTTL,
	})

	// Store the server answer
	if resp != nil {
		c.Resp = *resp
	}

	if err != nil {
		// glog.Infof("Error creating key: \"%s\" on the etcd server. Error: %s", dir+"/"+key, err)
		return err
	}
	// glog.Infof("Successfully created key: \"%s\" on the etcd server. The metadata is: %v", dir+"/"+key, c.Resp)

	return err

}

// Blocked wait while setting "key" to "value" under "dir" etcd directory. Block until successfully setting "key" to "value".
func (c *Myetcdclient) BlockforKey(dir, key, value string) error {

	var err error

	for {
		// glog.Infof(" ######> Attempting to uniquely create key: %s.", dir+"/"+key)
		err = c.CreateKey(dir, key, value)

	Retry:
		if err != nil {
			if err.(client.Error).Code == etcderr.EcodeNodeExist { // etcd error code "EcodeNodeExist" (i.e. Error code 105 /  "Key already exists")
				// Key  exists on server --  blocked wait on it
				glog.Infof("Key: \"%s\" already exists on the server. Watching... ", dir+"/"+key)

				// Create watcher
				watcher := c.kapi.Watcher(dir+"/"+key, &client.WatcherOptions{
					AfterIndex: 0,
					Recursive:  false,
				})

				// Waiting for key -- Loop....
				var r *client.Response
				var e error
				for {
					r, e = watcher.Next(context.TODO())
					if e != nil {
						glog.Fatalf("Fatal error in watching for key: %s. Error: %s", dir+"/"+key, e)
					}
					// glog.Infof(" ===> Notification while watching for key: %s. Server response:  %#v", dir+"/"+key, *r)
					// Once we get a notification about the leader key, try to set it again
					switch r.Action {
					case "delete", "expire": // Old key has expired. Attempt an atomic "Compare-and-Swap" with the old key value contained in the response
						// XXX -- Wrong: Since the key has expired, "conservative" Compare-and-Swap (above) will return an error since the key doesn't exist anylonger.
						// Use "CreateKey" instead but leaving the code flow in place in case we need to change it by changing the behaviour of CASKey(...)
						// err = c.CASKey(dir, key, r.Node.Value, value)
						err = c.CreateKey(dir, key, value)
						goto Retry
					case "compareAndSwap": // curent leader is updating the key, continue watching the key...
						continue

					// Don't rember when was this used for...
					// case "update":
					// ???
					//

					default:
						glog.Fatalf("===> Don't know how to process notification while watching for key: %s. Server response:  %#v", dir+"/"+key, *r)
					}
				}
			} else {
				// Unexpected error from the etcd server. Bail out.
				return err
			}

		} else { // We could set the key successfully
			return nil
		}
	}
}

// CAS (Compare-and-Swap) key on the server
func (c *Myetcdclient) CASKey(dir, key, oldvalue, newvalue string) error {

	resp, err := c.kapi.Set(context.Background(), dir+"/"+key, newvalue, &client.SetOptions{
		Dir:       false,
		PrevExist: client.PrevExist,
		PrevValue: oldvalue,
		TTL:       ServiceTTL,
	})

	// Store the server answer
	if resp != nil {
		c.Resp = *resp
	}

	if err != nil {
		if err.(client.Error).Code == etcderr.EcodeKeyNotFound { // etcd error code "EcodeKeyNotFound" (i.e. Error code 100 /  "Key not found")
			// etcd error code "EcodeKeyNotFound" (i.e. Error code 100 /  "Key not found")
			// This should _NOT_ happen. Two alternatives 1) Quietly re-set the key 2) Be conservative and bail out.

			// Alternative 1:
			// return c.CreateKey(dir, key, newvalue)

			// Alternative 2: Be conservative and bail out
			glog.Fatalf("===> FATAL: Compare-and-swap (CAS) key: \"%s\" on the etcd server failed: %s", dir+"/"+key, err)

		} else {
			return err
		}
	}
	// glog.Infof("Success in atomically updating (CAS) my key to: \"%s\" on the etcd server. The metadata is: %v", dir+"/"+key, c.Resp)

	return nil

}

// Renew a (previously) existing "key" with "value" under "dir"
func (c *Myetcdclient) RenewKey(dir, key, value string) error {
	for {
		err := c.CASKey(dir, key, value, value)
		if err != nil {
			return err
		}
		// glog.Infof("Successfully renewed key: \"%s\" on the etcd server", dir+"/"+key)

		// Sleep for min (1, ServiceTTL/NrClients) seconds

		if time.Second > (ServiceTTL / NrClients) {
			// glog.Info("Loooping...Waking up again in 1 second")
			time.Sleep(time.Second)
		} else {
			// glog.Infof("Loooping...Waking up again in %v seconds ", time.Duration(ServiceTTL/NrClients))
			time.Sleep(ServiceTTL / NrClients)
		}

	}
}

// Attempt to get the given key from the etcd server. If we get an valid answer, then store the response metadata
func (c *Myetcdclient) GetKey(key string) error {

	resp, err := c.kapi.Get(context.Background(), key, nil)
	// Store the server answer metadata for later
	if resp != nil {
		c.Resp = *resp
	}

	if err != nil {
		glog.Infof("Error getting key: \"%s\" from the etcd server. Error: %s", key, err)
		return err
	}
	// glog.Infof("Success in getting key \"%s\" from the etcd server. The metadata is: %v", key, c.Resp)

	return err

}

////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////
////////////////////////////////////////////////////////////////////////////////

func LeaderElection() {

	glog.Infof("The etcd server URLs are: %v", k8sMasterConfig.EtcdClientInfo.EtcdServerUrls)

	// create etcd client
	myc := &Myetcdclient{
	// cancel: make(chan struct{}),
	}

	// build an etcd KeysAPI for interacting with the etcd server

	// XXX - Still TODO: Use X509 certificates, if any were given

	myc.Config = client.Config{
		Endpoints:               k8sMasterConfig.EtcdClientInfo.EtcdServerUrls,
		Transport:               client.DefaultTransport,
		HeaderTimeoutPerRequest: time.Second,
	}

	conn, err := client.New(myc.Config)

	if err != nil {
		glog.Fatalf("Error creating etcd client. Error: %s", err)
	}

	myc.kapi = client.NewKeysAPI(conn)

	// XXX -- create a key for this host with a lifetime of "ServiceTTL"  under the "/hosts/<hostname>" subdir of the given directory (which is created if doesn't exist).

	hname, _ := os.Hostname()
	dir := Topdir + "/hosts"

	if err := myc.CreateKey(dir, hname, hname); err != nil {
		// Check if this an etcd server error or other type
		switch err.(type) {
		case client.Error:
			if err.(client.Error).Code == etcderr.EcodeNodeExist { // etcd error code "EcodeNodeExist" (i.e. Error code 105 /  "Key already exists")
				glog.Warningf("Host \"%s\" already registered on the etcd server. Waiting %d seconds for gracious exit (etcd key TTL)", hname, ServiceTTL/time.Second)
				time.Sleep(ServiceTTL)
				os.Exit(0)
			} else {
				glog.Fatalf("Error creating key: \"%s\" on the etcd server. Error: %s", dir+"/"+hname, err)
			}

		default:
			glog.Warningf("etcd server error, aborting... Error: %s", err)
			os.Exit(0)
		}

	} else { // Keep renewing the key for this node
		glog.Infof("Successfully registered node \"%s\" on etcd servers: %v", hname, k8sMasterConfig.EtcdClientInfo.EtcdServerUrls)
		go myc.RenewKey(dir, hname, hname)
	}

	// Try to get a leader lease by creating ".../leader" key on etcd server. The value of the key is the local hostname.

	err = myc.BlockforKey(Topdir, "leader", hname)

	if err != nil {
		glog.Fatalf("===> Fatal error in trying to get leader lease Error: %s", err)
		//....
	}

	go myc.RenewKey(Topdir, "leader", hname)

	glog.Info(" ######## Successfully got a leader lock ######## ")

}
