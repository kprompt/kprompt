package linkerd

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	ServerGroup   = "policy.linkerd.io"
	ServerKind    = "Server"
	ServerCRDName = "servers.policy.linkerd.io"
)

// Availability is the Linkerd detect result (policy Server CRD).
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether the Linkerd Server API is served in the cluster.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.LinkerdServerCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   ServerCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// DetailLabel formats availability for kprompt tools / learn output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "Linkerd Server CRD not found (policy.linkerd.io/Server)"
	}
	if len(av.Versions) == 0 {
		return "Linkerd Server CRD present"
	}
	return fmt.Sprintf("Linkerd Server CRD present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is actionable guidance when Linkerd is absent.
func InstallHint() string {
	return "Default: linkerd install --crds | kubectl apply -f - && linkerd install | kubectl apply -f - (https://linkerd.io/2/getting-started/). setup does not install Linkerd."
}
