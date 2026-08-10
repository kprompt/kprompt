package certmanager

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	CertificateGroup   = "cert-manager.io"
	CertificateKind    = "Certificate"
	CertificateCRDName = "certificates.cert-manager.io"
)

// Availability is the cert-manager detect result.
type Availability struct {
	Installed bool
	Group     string
	Kind      string
	CRDName   string
	Versions  []string
}

// Detect checks whether the cert-manager Certificate API is served.
func Detect(ctx context.Context, cfg *rest.Config) (Availability, error) {
	st, err := cluster.CertificateCRDStatus(ctx, cfg)
	if err != nil {
		return Availability{}, err
	}
	return Availability{
		Installed: st.Found,
		Group:     st.Group,
		Kind:      st.Kind,
		CRDName:   CertificateCRDName,
		Versions:  append([]string(nil), st.Versions...),
	}, nil
}

// DetailLabel formats availability for kprompt tools / learn output.
func DetailLabel(av Availability) string {
	if !av.Installed {
		return "Certificate CRD not found (cert-manager.io/Certificate)"
	}
	if len(av.Versions) == 0 {
		return "cert-manager Certificate CRD present"
	}
	return fmt.Sprintf("cert-manager Certificate CRD present (%s/%s: %s)", av.Group, av.Kind, strings.Join(av.Versions, ", "))
}

// InstallHint is actionable guidance when cert-manager is absent.
func InstallHint() string {
	return "Default: kubectl apply -f https://github.com/cert-manager/cert-manager/releases/download/v1.16.2/cert-manager.yaml (https://cert-manager.io/docs/installation/). setup does not install cert-manager."
}
