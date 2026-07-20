package tools

import (
	"context"
	"fmt"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/tools/argo"
	"github.com/kprompt/kprompt/internal/tools/tekton"
)

type kubeConnector interface {
	Connect(contextName string) (*cluster.Clients, error)
}

type defaultKube struct{}

func (defaultKube) Connect(contextName string) (*cluster.Clients, error) {
	return cluster.Connect(contextName)
}

func detectKubernetes(ctx context.Context, kubeCtx string, k kubeConnector) Result {
	r := Result{
		ID:           IDKubernetes,
		Name:         "Kubernetes",
		Capabilities: []Capability{CapQuery, CapMutate},
	}
	cl, err := k.Connect(kubeCtx)
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = err.Error()
		r.Hint = MissingHint(IDKubernetes)
		return r
	}
	r.Status = StatusAvailable
	if cl.Context != "" {
		r.Detail = "context: " + cl.Context
	} else {
		r.Detail = "kubeconfig connected"
	}
	return r
}

func detectArgoWorkflows(ctx context.Context, settings Settings, kubeCtx string, k kubeConnector) Result {
	r := Result{
		ID:           IDArgoWorkflows,
		Name:         "Argo Workflows",
		Capabilities: []Capability{CapSubmit, CapQuery, CapMutate},
	}
	if !settings.ArgoEnabled {
		r.Status = StatusDisabled
		r.Detail = "disabled in config or KPROMPT_ARGO_WORKFLOWS_ENABLED=0"
		r.Hint = argo.InstallHint()
		return r
	}
	cl, err := k.Connect(kubeCtx)
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = err.Error()
		r.Hint = MissingHint(IDKubernetes)
		return r
	}
	av, err := argo.Detect(ctx, cl.Config)
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = err.Error()
		r.Hint = argo.InstallHint()
		return r
	}
	if !av.Installed {
		r.Status = StatusUnavailable
		r.Detail = argo.DetailLabel(av)
		r.Hint = argo.InstallHint()
		return r
	}
	r.Status = StatusAvailable
	r.Detail = argo.DetailLabel(av)
	return r
}

func detectTekton(ctx context.Context, settings Settings, kubeCtx string, k kubeConnector) Result {
	r := Result{
		ID:           IDTekton,
		Name:         "Tekton",
		Capabilities: []Capability{CapSubmit, CapQuery, CapMutate},
	}
	if !settings.TektonEnabled {
		r.Status = StatusDisabled
		r.Detail = "disabled in config or KPROMPT_TEKTON_ENABLED=0"
		r.Hint = tekton.InstallHint()
		return r
	}
	cl, err := k.Connect(kubeCtx)
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = err.Error()
		r.Hint = MissingHint(IDKubernetes)
		return r
	}
	av, err := tekton.Detect(ctx, cl.Config)
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = err.Error()
		r.Hint = tekton.InstallHint()
		return r
	}
	if !av.Installed {
		r.Status = StatusUnavailable
		r.Detail = tekton.DetailLabel(av)
		r.Hint = tekton.InstallHint()
		return r
	}
	r.Status = StatusAvailable
	r.Detail = tekton.DetailLabel(av)
	return r
}

// RequireArgoWorkflows ensures the Workflow CRD is served in the active cluster.
func RequireArgoWorkflows(ctx context.Context, kubeCtx string, k kubeConnector) error {
	if k == nil {
		k = defaultKube{}
	}
	cl, err := k.Connect(kubeCtx)
	if err != nil {
		return cluster.Friendlier(fmt.Errorf("kubernetes: %w", err))
	}
	return argo.Require(ctx, cl.Config)
}

// RequireTekton ensures the PipelineRun CRD is served in the active cluster.
func RequireTekton(ctx context.Context, kubeCtx string, k kubeConnector) error {
	if k == nil {
		k = defaultKube{}
	}
	cl, err := k.Connect(kubeCtx)
	if err != nil {
		return cluster.Friendlier(fmt.Errorf("kubernetes: %w", err))
	}
	return tekton.Require(ctx, cl.Config)
}
