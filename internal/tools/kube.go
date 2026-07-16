package tools

import (
	"context"

	"github.com/kprompt/kprompt/internal/cluster"
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
		r.Hint = MissingHint(IDArgoWorkflows)
		return r
	}
	cl, err := k.Connect(kubeCtx)
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = err.Error()
		r.Hint = MissingHint(IDKubernetes)
		return r
	}
	ok, err := cluster.HasWorkflowCRD(ctx, cl.Config)
	if err != nil {
		r.Status = StatusUnavailable
		r.Detail = err.Error()
		r.Hint = MissingHint(IDArgoWorkflows)
		return r
	}
	if !ok {
		r.Status = StatusUnavailable
		r.Detail = "Workflow CRD not found (argoproj.io/Workflow)"
		r.Hint = MissingHint(IDArgoWorkflows)
		return r
	}
	r.Status = StatusAvailable
	r.Detail = "Workflow CRD present"
	return r
}
