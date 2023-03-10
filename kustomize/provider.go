package kustomize

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/discovery/cached/memory"
	"k8s.io/client-go/dynamic"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	apimachineryschema "k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/restmapper"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/mitchellh/go-homedir"
)

// Config ...
type Config struct {
	Client                dynamic.Interface
	Mapper                *restmapper.DeferredDiscoveryRESTMapper
	Mutex                 *sync.Mutex
	GzipLastAppliedConfig bool
}

// Provider ...
func Provider() *schema.Provider {
	p := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"kustomization_resource": kustomizationResource(),
		},

		DataSourcesMap: map[string]*schema.Resource{
			// legacy name of the data source
			"kustomization": dataSourceKustomization(),
			// new name for the data source
			"kustomization_build": dataSourceKustomization(),

			// define overlay from TF
			"kustomization_overlay": dataSourceKustomizationOverlay(),
		},

		Schema: map[string]*schema.Schema{
			"kubeconfig_path": {
				Type:         schema.TypeString,
				Optional:     true,
				DefaultFunc:  schema.EnvDefaultFunc("KUBECONFIG_PATH", nil),
				ExactlyOneOf: []string{"kubeconfig_path", "kubeconfig_raw", "kubeconfig_incluster", "host"},
				Description:  "Path to a kubeconfig file. Can be set using KUBECONFIG_PATH env var",
			},
			"kubeconfig_raw": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"kubeconfig_path", "kubeconfig_raw", "kubeconfig_incluster", "host"},
				Description:  "Raw kube config. If kubeconfig_raw is set, KUBECONFIG_PATH is ignored.",
			},
			"kubeconfig_incluster": {
				Type:         schema.TypeBool,
				Optional:     true,
				ExactlyOneOf: []string{"kubeconfig_path", "kubeconfig_raw", "kubeconfig_incluster", "host"},
				Description:  "Set to true when running inside a kubernetes cluster. If kubeconfig_incluster is set, KUBECONFIG_PATH is ignored.",
			},
			"context": {
				Type:        schema.TypeString,
				Optional:    true,
				DefaultFunc: schema.EnvDefaultFunc("KUBECONFIG_CONTEXT", nil),
				Description: "Context to use in kubeconfig with multiple contexts, if not specified the default context is to be used.",
			},
			"host": {
				Type:         schema.TypeString,
				Optional:     true,
				ExactlyOneOf: []string{"kubeconfig_path", "kubeconfig_raw", "kubeconfig_incluster", "host"},
				RequiredWith: []string{"host", "cluster_ca_certificate", "token"},
				Description:  "The hostname (in form of URI) of Kubernetes master.",
			},
			"cluster_ca_certificate": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"host", "cluster_ca_certificate", "token"},
				Description:  "PEM-encoded root certificates bundle for TLS authentication.",
			},
			"token": {
				Type:         schema.TypeString,
				Optional:     true,
				RequiredWith: []string{"host", "cluster_ca_certificate", "token"},
				Description:  "Token to authentifcate an service account",
			},
			"gzip_last_applied_config": {
				Type:        schema.TypeBool,
				Optional:    true,
				Default:     true,
				Description: "When 'true' compress the lastAppliedConfig annotation for resources that otherwise would exceed K8s' max annotation size. All other resources use the regular uncompressed annotation. Set to 'false' to disable compression entirely.",
			},
		},
	}

	p.ConfigureFunc = func(d *schema.ResourceData) (interface{}, error) {
		var config *rest.Config
		var err error

		raw := d.Get("kubeconfig_raw").(string)
		path := d.Get("kubeconfig_path").(string)
		incluster := d.Get("kubeconfig_incluster").(bool)
		context := d.Get("context").(string)
		host := d.Get("host").(string)
		cluster_ca_certificate := d.Get("cluster_ca_certificate").(string)
		token := d.Get("token").(string)

		if raw != "" {
			config, err = getClientConfig([]byte(raw), context)
			if err != nil {
				return nil, fmt.Errorf("provider kustomization: kubeconfig_raw: %s", err)
			}
		}

		if raw == "" && path != "" {
			data, err := readKubeconfigFile(path)
			if err != nil {
				return nil, fmt.Errorf("provider kustomization: kubeconfig_path: %s", err)
			}

			config, err = getClientConfig(data, context)
			if err != nil {
				return nil, fmt.Errorf("provider kustomization: kubeconfig_path: %s", err)
			}
		}

		if host != "" && cluster_ca_certificate != "" && token != "" {
			loader := &clientcmd.ClientConfigLoadingRules{}
			overrides := &clientcmd.ConfigOverrides{}
			phost, _, err := rest.DefaultServerURL(host, "", apimachineryschema.GroupVersion{}, true)
			if err != nil {
				return nil, fmt.Errorf("provider kustomization: host: %s", err)
			}
			overrides.ClusterInfo.Server = phost.String()
			overrides.ClusterInfo.CertificateAuthorityData = bytes.NewBufferString(cluster_ca_certificate).Bytes()
			overrides.AuthInfo.Token = token
			config, err = clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loader, overrides).ClientConfig()
			if err != nil {
				return nil, fmt.Errorf("provider kustomization: host, cluster_ca_certificate, token: %s", err)
			}

		}

		if incluster {
			config, err = rest.InClusterConfig()
			if err != nil {
				return nil, fmt.Errorf("provider kustomization: couldn't load in cluster config: %s", err)
			}
		}

		// empty default config required to support
		// using a cluster resource or data source
		// that may not exist yet, to configure the provider
		if config == nil {
			config = &rest.Config{}
		}

		// Increase QPS and Burst rate limits
		config.QPS = 120
		config.Burst = 240

		client, err := dynamic.NewForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("provider kustomization: %s", err)
		}

		dc, err := discovery.NewDiscoveryClientForConfig(config)
		if err != nil {
			return nil, fmt.Errorf("provider kustomization: %s", err)
		}

		mapper := restmapper.NewDeferredDiscoveryRESTMapper(memory.NewMemCacheClient(dc))

		// Mutex to prevent parallel Kustomizer runs
		// temp workaround for upstream bug
		// https://github.com/kubernetes-sigs/kustomize/issues/3659
		mu := &sync.Mutex{}

		gzipLastAppliedConfig := d.Get("gzip_last_applied_config").(bool)

		return &Config{client, mapper, mu, gzipLastAppliedConfig}, nil
	}

	return p
}

func readKubeconfigFile(s string) ([]byte, error) {
	p, err := homedir.Expand(s)
	if err != nil {
		return nil, err
	}

	data, err := ioutil.ReadFile(p)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func getClientConfig(data []byte, context string) (*rest.Config, error) {
	if len(context) == 0 {
		return clientcmd.RESTConfigFromKubeConfig(data)
	}

	rawConfig, err := clientcmd.Load(data)
	if err != nil {
		return nil, err
	}

	var clientConfig clientcmd.ClientConfig = clientcmd.NewNonInteractiveClientConfig(
		*rawConfig,
		context,
		&clientcmd.ConfigOverrides{CurrentContext: context},
		nil)

	return clientConfig.ClientConfig()
}
