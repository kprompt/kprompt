package cluster

import (
	"context"
	"fmt"
	"sort"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

// CRDStatus describes whether an API kind is served and which versions exist.
type CRDStatus struct {
	Found    bool
	Group    string
	Kind     string
	Versions []string
}

// ScaledObjectCRDStatus reports KEDA ScaledObject API availability in the cluster.
func ScaledObjectCRDStatus(ctx context.Context, cfg *rest.Config) (CRDStatus, error) {
	_ = ctx
	st := CRDStatus{Group: "keda.sh", Kind: "ScaledObject"}
	versions, found, err := servedKindVersions(cfg, st.Group, st.Kind)
	if err != nil {
		return st, err
	}
	st.Found = found
	st.Versions = versions
	return st, nil
}

// HasScaledObjectCRD reports whether the KEDA ScaledObject API is served.
func HasScaledObjectCRD(ctx context.Context, cfg *rest.Config) (bool, error) {
	st, err := ScaledObjectCRDStatus(ctx, cfg)
	if err != nil {
		return false, err
	}
	return st.Found, nil
}

// PipelineRunCRDStatus reports Tekton PipelineRun API availability in the cluster.
func PipelineRunCRDStatus(ctx context.Context, cfg *rest.Config) (CRDStatus, error) {
	_ = ctx
	st := CRDStatus{Group: "tekton.dev", Kind: "PipelineRun"}
	versions, found, err := servedKindVersions(cfg, st.Group, st.Kind)
	if err != nil {
		return st, err
	}
	st.Found = found
	st.Versions = versions
	return st, nil
}

// HasPipelineRunCRD reports whether the Tekton PipelineRun API is served.
func HasPipelineRunCRD(ctx context.Context, cfg *rest.Config) (bool, error) {
	st, err := PipelineRunCRDStatus(ctx, cfg)
	if err != nil {
		return false, err
	}
	return st.Found, nil
}

// WorkflowCRDStatus reports Argo Workflows API availability in the cluster.
func WorkflowCRDStatus(ctx context.Context, cfg *rest.Config) (CRDStatus, error) {
	_ = ctx
	st := CRDStatus{Group: "argoproj.io", Kind: "Workflow"}
	versions, found, err := servedKindVersions(cfg, st.Group, st.Kind)
	if err != nil {
		return st, err
	}
	st.Found = found
	st.Versions = versions
	return st, nil
}

// HasWorkflowCRD reports whether the Argo Workflows API (Workflow kind) is served.
func HasWorkflowCRD(ctx context.Context, cfg *rest.Config) (bool, error) {
	st, err := WorkflowCRDStatus(ctx, cfg)
	if err != nil {
		return false, err
	}
	return st.Found, nil
}

func servedKindVersions(cfg *rest.Config, group, kind string) ([]string, bool, error) {
	if cfg == nil {
		return nil, false, fmt.Errorf("rest config is nil")
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, false, err
	}
	groups, err := dc.ServerGroups()
	if err != nil {
		return nil, false, err
	}
	var versions []string
	found := false
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
				return nil, false, err
			}
			for _, r := range res.APIResources {
				if r.Kind == kind {
					found = true
					versions = append(versions, v.Version)
				}
			}
		}
	}
	sort.Strings(versions)
	return versions, found, nil
}
