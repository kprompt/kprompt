package cluster

import (
	"fmt"

	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// DynamicForConfig builds a dynamic client for generic resource access (T-050).
func DynamicForConfig(cfg *rest.Config) (dynamic.Interface, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rest config is nil")
	}
	return dynamic.NewForConfig(cfg)
}
