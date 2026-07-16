package cluster

import (
	"context"
	"fmt"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// HasWorkflowCRD reports whether the Argo Workflows API (Workflow kind) is served.
func HasWorkflowCRD(ctx context.Context, cfg *rest.Config) (bool, error) {
	return hasServedKind(ctx, cfg, "argoproj.io", "Workflow")
}

func hasServedKind(ctx context.Context, cfg *rest.Config, group, kind string) (bool, error) {
	if cfg == nil {
		return false, fmt.Errorf("rest config is nil")
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return false, err
	}
	groups, err := dc.ServerGroups()
	if err != nil {
		return false, err
	}
	for _, g := range groups.Groups {
		if g.Name != group {
			continue
		}
		for _, v := range g.Versions {
			res, err := dc.ServerResourcesForGroupVersion(v.GroupVersion)
			if err != nil {
				if discovery.IsGroupDiscoveryFailedError(err) {
					continue
				}
				return false, err
			}
			for _, r := range res.APIResources {
				if r.Kind == kind {
					return true, nil
				}
			}
		}
	}
	return false, nil
}
