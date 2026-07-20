package crossplane

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	XRDGroup   = "apiextensions.crossplane.io"
	XRDKind    = "CompositeResourceDefinition"
	XRDCRDName = "compositeresourcedefinitions.apiextensions.crossplane.io"
)

// Availability is the Crossplane integration detect result.
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether CompositeResourceDefinition is served (Crossplane core).
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.CompositeResourceDefinitionCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   XRDCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// Require returns a clear error when Crossplane is not installed.
func Require(ctx context.Context, cfg *rest.Config) error {
	av, err := Detect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("crossplane detect: %w", err)
	}
	if av.Installed {
		return nil
	}
	return NotInstalledError{Detail: "CompositeResourceDefinition CRD not found (apiextensions.crossplane.io)"}
}

// DetailLabel formats availability for kprompt tools output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "CompositeResourceDefinition CRD not found (apiextensions.crossplane.io)"
	}
	if len(av.Versions) == 0 {
		return "Crossplane XRD API present"
	}
	return fmt.Sprintf("Crossplane XRD API present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is the actionable install guidance for operators.
func InstallHint() string {
	return "Install Crossplane in the cluster (https://docs.crossplane.io/latest/software/install/) and configure Providers/Compositions before provisioning claims."
}

// NotInstalledError is returned when Crossplane operations are requested without Crossplane.
type NotInstalledError struct {
	Detail string
}

func (e NotInstalledError) Error() string {
	msg := strings.TrimSpace(e.Detail)
	if msg == "" {
		msg = "Crossplane not installed"
	}
	return fmt.Sprintf("Crossplane is not available: %s. %s", msg, InstallHint())
}
