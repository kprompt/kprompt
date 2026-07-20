package gitops

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	FluxGroup   = "kustomize.toolkit.fluxcd.io"
	FluxKind    = "Kustomization"
	FluxCRDName = "kustomizations.kustomize.toolkit.fluxcd.io"

	ArgoCDGroup   = "argoproj.io"
	ArgoCDKind    = "Application"
	ArgoCDCRDName = "applications.argoproj.io"
)

// Availability is the GitOps integration detect result (Flux and/or Argo CD).
type Availability struct {
	Installed bool
	Flux      bool
	ArgoCD    bool
	Detail    string
}

// Detect checks whether Flux Kustomization and/or Argo CD Application APIs are served.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	flux, err := cluster.KustomizationCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	argo, err := cluster.ApplicationCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	av := Availability{
		Flux:   flux.Found,
		ArgoCD: argo.Found,
	}
	av.Installed = av.Flux || av.ArgoCD
	av.Detail = DetailLabel(av)
	return av, nil
}

// Require returns a clear error when neither Flux nor Argo CD is installed.
func Require(ctx context.Context, cfg *rest.Config) error {
	av, err := Detect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("gitops detect: %w", err)
	}
	if av.Installed {
		return nil
	}
	return NotInstalledError{Detail: "neither Flux Kustomization nor Argo CD Application CRD found"}
}

// DetailLabel formats availability for kprompt tools output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "Flux Kustomization / Argo CD Application CRDs not found"
	}
	parts := make([]string, 0, 2)
	if av.Flux {
		parts = append(parts, "Flux Kustomization")
	}
	if av.ArgoCD {
		parts = append(parts, "Argo CD Application")
	}
	return strings.Join(parts, " + ") + " present"
}

// InstallHint is the actionable install guidance for operators.
func InstallHint() string {
	return "Install Flux (https://fluxcd.io/flux/installation/) and/or Argo CD (https://argo-cd.readthedocs.io/en/stable/getting_started/) to manage GitOps sync and health."
}

// NotInstalledError is returned when GitOps operations are requested without Flux or Argo CD.
type NotInstalledError struct {
	Detail string
}

func (e NotInstalledError) Error() string {
	msg := strings.TrimSpace(e.Detail)
	if msg == "" {
		msg = "GitOps controllers not installed"
	}
	return fmt.Sprintf("GitOps is not available: %s. %s", msg, InstallHint())
}
