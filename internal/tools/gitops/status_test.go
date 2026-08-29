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

func TestResourceDriftsFromApp(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "shop", "namespace": "argocd"},
		"status": map[string]any{
			"resources": []any{
				map[string]any{"group": "apps", "version": "v1", "kind": "Deployment", "name": "api", "namespace": "shop", "status": "OutOfSync"},
				map[string]any{"group": "apps", "version": "v1", "kind": "Deployment", "name": "ok", "namespace": "shop", "status": "Synced"},
			},
		},
	}}
	got := resourceDriftsFromArgoApp(obj)
	if len(got) != 1 || got[0].Name != "api" || got[0].APIVersion != "apps/v1" {
		t.Fatalf("%+v", got)
	}
}

func TestResourceDriftsFromFluxInventory(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "apps", "namespace": "flux-system"},
		"status": map[string]any{
			"inventory": map[string]any{
				"entries": []any{
					map[string]any{"id": "shop_api_apps_Deployment", "v": "v1"},
					map[string]any{"id": "shop_api__Service", "v": "v1"},
					map[string]any{"id": "_widgets.example.com_apiextensions.k8s.io_CustomResourceDefinition", "v": "v1"},
				},
			},
		},
	}}
	got, err := resourceDriftsFromFluxKustomization(obj)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("%+v", got)
	}
	if got[0].Kind != "Deployment" || got[0].Name != "api" || got[0].Namespace != "shop" || got[0].APIVersion != "apps/v1" {
		t.Fatalf("dep=%+v", got[0])
	}
	if got[1].Kind != "Service" || got[1].Name != "api" || got[1].APIVersion != "v1" {
		t.Fatalf("svc=%+v", got[1])
	}
	if got[2].Kind != "CustomResourceDefinition" || got[2].Name != "widgets.example.com" || got[2].Namespace != "" {
		t.Fatalf("crd=%+v", got[2])
	}
	if got[0].Status != "OutOfSync" {
		t.Fatalf("status=%q", got[0].Status)
	}
}

func TestResourceDriftsFromFluxMissingInventory(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "apps", "namespace": "flux-system"},
		"status":   map[string]any{},
	}}
	_, err := resourceDriftsFromFluxKustomization(obj)
	if err == nil || !strings.Contains(err.Error(), "inventory unavailable") {
		t.Fatalf("err=%v", err)
	}
}

func TestParseFluxInventoryID(t *testing.T) {
	ns, name, group, kind, ok := parseFluxInventoryID("payments_redis_apps_Deployment")
	if !ok || ns != "payments" || name != "redis" || group != "apps" || kind != "Deployment" {
		t.Fatalf("%q %q %q %q %v", ns, name, group, kind, ok)
	}
	ns, name, group, kind, ok = parseFluxInventoryID("_cluster-ns__Namespace")
	if !ok || ns != "" || name != "cluster-ns" || group != "" || kind != "Namespace" {
		t.Fatalf("cluster %q %q %q %q %v", ns, name, group, kind, ok)
	}
}
