package gitops

import (
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	k8stesting "k8s.io/client-go/testing"

	"k8s.io/client-go/dynamic/fake"
)

func kustomizationObject(name, ns string, annotations map[string]any) *unstructured.Unstructured {
	meta := map[string]any{"name": name, "namespace": ns}
	if annotations != nil {
		meta["annotations"] = annotations
	}
	return &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata":   meta,
	}}
}

func applicationObject(name, ns string, history []any) *unstructured.Unstructured {
	obj := map[string]any{
		"apiVersion": "argoproj.io/v1alpha1",
		"kind":       "Application",
		"metadata":   map[string]any{"name": name, "namespace": ns},
	}
	if history != nil {
		obj["status"] = map[string]any{"history": history}
	}
	return &unstructured.Unstructured{Object: obj}
}

func TestTriggerSyncFluxAnnotates(t *testing.T) {
	obj := kustomizationObject("apps", "flux-system", map[string]any{"kept": "yes"})
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)

	res, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{
		Engine: "flux", Name: "apps", Namespace: "flux-system",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != "flux" || res.Kind != FluxKind || res.Name != "apps" || res.Namespace != "flux-system" || res.Action != "sync" {
		t.Fatalf("%+v", res)
	}

	updated, err := dc.Resource(KustomizationGVR).Namespace("flux-system").Get(context.Background(), "apps", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	anns := updated.GetAnnotations()
	if anns["kept"] != "yes" {
		t.Fatalf("expected pre-existing annotation preserved, got %+v", anns)
	}
	stamp, ok := anns["reconcile.fluxcd.io/requestedAt"]
	if !ok {
		t.Fatalf("expected reconcile annotation, got %+v", anns)
	}
	if _, err := time.Parse(time.RFC3339Nano, stamp); err != nil {
		t.Fatalf("annotation not RFC3339Nano: %q (%v)", stamp, err)
	}
}

func TestTriggerSyncArgoCDUpdatesOperationAndAnnotation(t *testing.T) {
	obj := applicationObject("payments", "argocd", nil)
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)

	res, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{
		Engine: "argocd", Name: "payments", Namespace: "argocd", Revision: "v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != "argocd" || res.Kind != ArgoCDKind || !strings.Contains(res.Message, "v2") {
		t.Fatalf("%+v", res)
	}

	updated, err := dc.Resource(ApplicationGVR).Namespace("argocd").Get(context.Background(), "payments", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetAnnotations()["argocd.argoproj.io/refresh"] != "hard" {
		t.Fatalf("expected hard-refresh annotation, got %+v", updated.GetAnnotations())
	}
	rev, ok, _ := unstructured.NestedString(updated.Object, "operation", "sync", "revision")
	if !ok || rev != "v2" {
		t.Fatalf("expected operation.sync.revision=v2, got ok=%v rev=%q", ok, rev)
	}
	user, ok, _ := unstructured.NestedString(updated.Object, "operation", "initiatedBy", "username")
	if !ok || user != "kprompt" {
		t.Fatalf("expected operation.initiatedBy.username=kprompt, got ok=%v user=%q", ok, user)
	}
}

func TestTriggerSyncArgoCDFallsBackToPatchOnUpdateError(t *testing.T) {
	obj := applicationObject("payments", "argocd", nil)
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)
	dc.PrependReactor("update", "applications", func(action k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, apiServerBusyErr{}
	})

	res, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{
		Engine: "argocd", Name: "payments", Namespace: "argocd", Revision: "v3",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Message, "v3") {
		t.Fatalf("%+v", res)
	}

	updated, err := dc.Resource(ApplicationGVR).Namespace("argocd").Get(context.Background(), "payments", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetAnnotations()["argocd.argoproj.io/refresh"] != "hard" {
		t.Fatalf("expected hard-refresh annotation via patch, got %+v", updated.GetAnnotations())
	}
	rev, ok, _ := unstructured.NestedString(updated.Object, "operation", "sync", "revision")
	if !ok || rev != "v3" {
		t.Fatalf("expected operation.sync.revision=v3 via patch, got ok=%v rev=%q", ok, rev)
	}
}

type apiServerBusyErr struct{}

func (apiServerBusyErr) Error() string { return "server busy" }

func TestTriggerSyncArgoCDRollbackUsesPriorHistoryRevision(t *testing.T) {
	history := []any{
		map[string]any{"revision": "v1"},
		map[string]any{"revision": "v2"},
	}
	obj := applicationObject("payments", "argocd", history)
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)

	res, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{
		Engine: "argocd", Name: "payments", Namespace: "argocd", Action: "rollback",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Action != "rollback" || !strings.Contains(res.Message, "v1") {
		t.Fatalf("expected rollback to prior revision v1, got %+v", res)
	}
}

func TestTriggerSyncArgoCDRollbackExplicitRevisionWins(t *testing.T) {
	history := []any{
		map[string]any{"revision": "v1"},
		map[string]any{"revision": "v2"},
	}
	obj := applicationObject("payments", "argocd", history)
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)

	res, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{
		Engine: "argocd", Name: "payments", Namespace: "argocd", Action: "rollback", Revision: "v9",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Message, "v9") {
		t.Fatalf("expected explicit revision to win, got %+v", res)
	}
}

func TestTriggerSyncDefaultsNamespaceAndAction(t *testing.T) {
	obj := kustomizationObject("apps", "default", nil)
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)

	res, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{Engine: "flux", Name: "apps"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Namespace != "default" || res.Action != "sync" {
		t.Fatalf("%+v", res)
	}
}

func TestTriggerSyncArgoAliasEngineName(t *testing.T) {
	obj := applicationObject("payments", "default", nil)
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme(), obj)

	res, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{Engine: "argo", Name: "payments"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Engine != "argocd" {
		t.Fatalf("%+v", res)
	}
}

func TestTriggerSyncRequiresName(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	_, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{Engine: "flux"})
	if err == nil || !strings.Contains(err.Error(), "requires a named") {
		t.Fatalf("err=%v", err)
	}
}

func TestTriggerSyncRejectsAutoEngine(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	_, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{Name: "apps", Engine: "auto"})
	if err == nil || !strings.Contains(err.Error(), "requires engine=flux or engine=argocd") {
		t.Fatalf("err=%v", err)
	}
}

func TestTriggerSyncRejectsEmptyEngine(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	_, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{Name: "apps"})
	if err == nil || !strings.Contains(err.Error(), "requires engine=flux or engine=argocd") {
		t.Fatalf("err=%v", err)
	}
}

func TestTriggerSyncRejectsUnsupportedEngine(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	_, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{Name: "apps", Engine: "spinnaker"})
	if err == nil || !strings.Contains(err.Error(), `unsupported gitops engine "spinnaker"`) {
		t.Fatalf("err=%v", err)
	}
}

func TestTriggerSyncFluxNotFound(t *testing.T) {
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	_, err := TriggerSyncWithClient(context.Background(), dc, SyncRequest{Engine: "flux", Name: "missing", Namespace: "flux-system"})
	if err == nil || !strings.Contains(err.Error(), "get kustomization") {
		t.Fatalf("err=%v", err)
	}
}

func TestTriggerSyncNilConfig(t *testing.T) {
	_, err := TriggerSync(context.Background(), nil, SyncRequest{Engine: "flux", Name: "apps"})
	if err == nil || !strings.Contains(err.Error(), "rest config is nil") {
		t.Fatalf("err=%v", err)
	}
}

func TestSyncResultLabel(t *testing.T) {
	res := SyncResult{Engine: "flux", Kind: FluxKind, Name: "apps", Message: "requested Flux reconcile (sync)"}
	if got := res.Label(); !strings.Contains(got, "flux") || !strings.Contains(got, "apps") {
		t.Fatalf("label=%q", got)
	}
}
