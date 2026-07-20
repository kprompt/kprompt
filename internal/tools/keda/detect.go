package keda

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	ScaledObjectGroup   = "keda.sh"
	ScaledObjectKind    = "ScaledObject"
	ScaledObjectCRDName = "scaledobjects.keda.sh"
)

// Availability is the KEDA integration detect result.
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether the ScaledObject API is served in the cluster.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.ScaledObjectCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   ScaledObjectCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// Require returns a clear error when KEDA is not installed.
func Require(ctx context.Context, cfg *rest.Config) error {
	av, err := Detect(ctx, cfg)
	if err != nil {
		return fmt.Errorf("keda detect: %w", err)
	}
	if av.Installed {
		return nil
	}
	return NotInstalledError{Detail: "ScaledObject CRD not found (keda.sh/ScaledObject)"}
}

// DetailLabel formats availability for kprompt tools output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "ScaledObject CRD not found (keda.sh/ScaledObject)"
	}
	if len(av.Versions) == 0 {
		return "ScaledObject CRD present"
	}
	return fmt.Sprintf("ScaledObject CRD present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is the actionable install guidance for operators.
func InstallHint() string {
	return "Install KEDA in the cluster (https://keda.sh/docs/deploy/) or use plain Deployment scale without event triggers."
}

// NotInstalledError is returned when KEDA operations are requested without KEDA.
type NotInstalledError struct {
	Detail string
}

func (e NotInstalledError) Error() string {
	msg := strings.TrimSpace(e.Detail)
	if msg == "" {
		msg = "ScaledObject CRD not installed"
	}
	return fmt.Sprintf("KEDA is not available: %s. %s", msg, InstallHint())
}
