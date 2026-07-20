package tekton

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	PipelineGroup      = "tekton.dev"
	PipelineRunKind    = "PipelineRun"
	PipelineRunCRDName = "pipelineruns.tekton.dev"
)

// Availability is the Tekton integration detect result.
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether the PipelineRun API is served in the cluster.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.PipelineRunCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   PipelineRunCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// Require returns a clear error when Tekton is not installed.
func Require(ctx context.Context, cfg *rest.Config) error {
	av, err := Detect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("tekton detect: %w", err)
	}
	if av.Installed {
		return nil
	}
	return NotInstalledError{Detail: "PipelineRun CRD not found (tekton.dev/PipelineRun)"}
}

// DetailLabel formats availability for kprompt tools output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "PipelineRun CRD not found (tekton.dev/PipelineRun)"
	}
	if len(av.Versions) == 0 {
		return "PipelineRun CRD present"
	}
	return fmt.Sprintf("PipelineRun CRD present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is the actionable install guidance for operators.
func InstallHint() string {
	return "Install Tekton Pipelines in the cluster (https://tekton.dev/docs/getting-started/) or pick a Kubernetes-only prompt."
}

// NotInstalledError is returned when Tekton operations are requested without Tekton.
type NotInstalledError struct {
	Detail string
}

func (e NotInstalledError) Error() string {
	msg := strings.TrimSpace(e.Detail)
	if msg == "" {
		msg = "PipelineRun CRD not installed"
	}
	return fmt.Sprintf("Tekton is not available: %s. %s", msg, InstallHint())
}
