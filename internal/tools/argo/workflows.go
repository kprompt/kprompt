package argo

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	WorkflowGroup   = "argoproj.io"
	WorkflowKind    = "Workflow"
	WorkflowCRDName = "workflows.argoproj.io"
)

// Availability is the Argo Workflows integration detect result.
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether the Workflow API is served in the cluster.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.WorkflowCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   WorkflowCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// Require returns a clear error when Argo Workflows is not installed.
func Require(ctx context.Context, cfg *rest.Config) error {
	av, err := Detect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("argo workflows detect: %w", err)
	}
	if av.Installed {
		return nil
	}
	return NotInstalledError{Detail: "Workflow CRD not found (argoproj.io/Workflow)"}
}

// DetailLabel formats availability for kprompt tools output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "Workflow CRD not found (argoproj.io/Workflow)"
	}
	if len(av.Versions) == 0 {
		return "Workflow CRD present"
	}
	return fmt.Sprintf("Workflow CRD present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is the actionable install guidance for operators.
func InstallHint() string {
	return "Install Argo Workflows in the cluster (https://argo-workflows.readthedocs.io/en/latest/quick-start/) or pick a Kubernetes-only prompt."
}

// NotInstalledError is returned when workflow operations are requested without Argo.
type NotInstalledError struct {
	Detail string
}

func (e NotInstalledError) Error() string {
	msg := strings.TrimSpace(e.Detail)
	if msg == "" {
		msg = "Workflow CRD not installed"
	}
	return fmt.Sprintf("Argo Workflows is not available: %s. %s", msg, InstallHint())
}
