package istio

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	VirtualServiceGroup   = "networking.istio.io"
	VirtualServiceKind    = "VirtualService"
	VirtualServiceCRDName = "virtualservices.networking.istio.io"
)

// Availability is the Istio integration detect result.
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether the VirtualService API is served in the cluster.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.VirtualServiceCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   VirtualServiceCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// Require returns a clear error when Istio networking is not installed.
func Require(ctx context.Context, cfg *rest.Config) error {
	av, err := Detect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("istio detect: %w", err)
	}
	if av.Installed {
		return nil
	}
	return NotInstalledError{Detail: "VirtualService CRD not found (networking.istio.io/VirtualService)"}
}

// DetailLabel formats availability for kprompt tools output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "VirtualService CRD not found (networking.istio.io/VirtualService)"
	}
	if len(av.Versions) == 0 {
		return "VirtualService CRD present"
	}
	return fmt.Sprintf("VirtualService CRD present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is the actionable install guidance for operators.
func InstallHint() string {
	return "Install Istio in the cluster (https://istio.io/latest/docs/setup/install/) or use generic get for non-mesh traffic."
}

// NotInstalledError is returned when Istio operations are requested without Istio.
type NotInstalledError struct {
	Detail string
}

func (e NotInstalledError) Error() string {
	msg := strings.TrimSpace(e.Detail)
	if msg == "" {
		msg = "VirtualService CRD not installed"
	}
	return fmt.Sprintf("Istio is not available: %s. %s", msg, InstallHint())
}
