package gitops

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSummarizeFluxReady(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "apps", "namespace": "flux-system"},
		"status": map[string]any{
			"lastAppliedRevision": "main@sha1:abc",
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True", "message": "Applied revision"},
			},
		},
	}}
	st := summarizeFlux(obj)
	if st.Engine != "flux" || st.Health != "Healthy" || st.Sync != "Synced" {
		t.Fatalf("%+v", st)
	}
}

func TestSummarizeArgoCD(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "payments", "namespace": "argocd"},
		"status": map[string]any{
			"sync":   map[string]any{"status": "Synced", "revision": "abc123"},
			"health": map[string]any{"status": "Healthy"},
		},
	}}
	st := summarizeArgoCD(obj)
	if st.Engine != "argocd" || st.Sync != "Synced" || st.Health != "Healthy" || !strings.Contains(st.Revision, "abc") {
		t.Fatalf("%+v", st)
	}
}

func TestDetailLabel(t *testing.T) {
	if got := DetailLabel(Availability{}); !strings.Contains(got, "not found") {
		t.Fatalf("%s", got)
	}
	if got := DetailLabel(Availability{Installed: true, Flux: true, ArgoCD: true}); !strings.Contains(got, "Flux") || !strings.Contains(got, "Argo CD") {
		t.Fatalf("%s", got)
	}
}
