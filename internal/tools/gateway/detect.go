package gateway

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	GatewayGroup   = "gateway.networking.k8s.io"
	GatewayKind    = "Gateway"
	GatewayCRDName = "gateways.gateway.networking.k8s.io"
)

// Availability is the Gateway API detect result.
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether the Gateway API Gateway kind is served.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.GatewayCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   GatewayCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// DetailLabel formats availability for kprompt tools / learn output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "Gateway CRD not found (gateway.networking.k8s.io/Gateway)"
	}
	if len(av.Versions) == 0 {
		return "Gateway API Gateway CRD present"
	}
	return fmt.Sprintf("Gateway API Gateway CRD present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is actionable guidance when Gateway API is absent.
func InstallHint() string {
	return "Default CRDs: kubectl apply -f https://github.com/kubernetes-sigs/gateway-api/releases/download/v1.2.1/standard-install.yaml (https://gateway-api.sigs.k8s.io/), then install a controller (Istio, Contour, Envoy Gateway). setup does not install Gateway API."
}
