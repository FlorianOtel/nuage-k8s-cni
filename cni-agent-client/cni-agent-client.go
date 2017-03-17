package cni

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/golang/glog"

	agenttypes "github.com/OpenPlatformSDN/cni-plugin/nuage-cni-agent/types"
	"github.com/OpenPlatformSDN/nuage-k8s-cni/config"
)

var (
	AgentClient     *http.Client
	AgentServerPort string
)

func InitClient(conf *config.AgentConfig) error {

	// Pick up Agent server port from startup configuration
	AgentServerPort = conf.CniConfig.ServerPort

	certPool := x509.NewCertPool()

	if pemData, err := ioutil.ReadFile(conf.CniConfig.CaFile); err != nil {
		err = fmt.Errorf("Error loading CNI agent server CA certificate data from: %s. Error: %s", conf.CniConfig.CaFile, err)
		glog.Error(err)
		return err
	} else {
		certPool.AppendCertsFromPEM(pemData)
	}

	// configure a TLS client to use those certificates
	AgentClient = new(http.Client)
	*AgentClient = http.Client{
		Transport: &http.Transport{
			MaxIdleConns:    agenttypes.MAX_CONNS,
			IdleConnTimeout: agenttypes.MAX_IDLE,
			TLSClientConfig: &tls.Config{
				RootCAs: certPool,
				// InsecureSkipVerify: true, // In case we want to skip server verification
			},
		},
	}

	return nil
}
