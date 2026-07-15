package cluster

import (
	"fmt"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// Clients wraps a typed Kubernetes clientset with the active context name.
type Clients struct {
	Clientset *kubernetes.Clientset
	Context   string
	Config    *rest.Config
}

// Connect builds a clientset from kubeconfig, optionally selecting a context.
func Connect(contextName string) (*Clients, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	clientCfg := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)
	raw, err := clientCfg.RawConfig()
	if err != nil {
		return nil, fmt.Errorf("load kubeconfig: %w", err)
	}
	restCfg, err := clientCfg.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("kube client config: %w", err)
	}
	cs, err := kubernetes.NewForConfig(restCfg)
	if err != nil {
		return nil, fmt.Errorf("kubernetes clientset: %w", err)
	}
	ctx := raw.CurrentContext
	if contextName != "" {
		ctx = contextName
	}
	return &Clients{Clientset: cs, Context: ctx, Config: restCfg}, nil
}
