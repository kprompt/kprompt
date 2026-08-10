package tools

import (
	"strings"
	"testing"
)

func TestMissingHint(t *testing.T) {
	cases := []struct {
		id   ID
		subs []string
	}{
		{IDHelm, []string{"Helm", "kprompt setup", "minimal"}},
		{IDPrometheus, []string{"Prometheus", "kprompt setup", "prometheus"}},
		{IDGrafana, []string{"Grafana", "kprompt setup", "grafana"}},
		{IDOpenTelemetry, []string{"Trace", "kprompt setup", "opentelemetry"}},
		{IDKubernetes, []string{"Kubernetes", "kubeconfig", "does not create clusters"}},
		{ID("unknown-tool"), []string{"not available"}},
		{IDArgoWorkflows, []string{"Argo", "kprompt setup", "argo-workflows"}},
		{IDGitOps, []string{"Flux", "flux bootstrap", "Argo CD", "setup does not install GitOps"}},
		{IDTekton, []string{"tekton-releases", "setup does not install Tekton"}},
		{IDKEDA, []string{"kedacore", "helm install keda", "setup does not install KEDA"}},
		{IDIstio, []string{"istioctl", "setup does not install Istio"}},
		{IDLinkerd, []string{"linkerd install", "setup does not install Linkerd"}},
		{IDGatewayAPI, []string{"gateway-api", "standard-install.yaml", "setup does not install Gateway API"}},
		{IDCertManager, []string{"cert-manager.yaml", "setup does not install cert-manager"}},
		{IDCrossplane, []string{"crossplane-stable", "setup does not install Crossplane"}},
	}
	for _, tc := range cases {
		t.Run(string(tc.id), func(t *testing.T) {
			got := MissingHint(tc.id)
			if got == "" {
				t.Fatal("empty hint")
			}
			for _, sub := range tc.subs {
				if !strings.Contains(got, sub) {
					t.Fatalf("hint %q missing %q", got, sub)
				}
			}
		})
	}
}
