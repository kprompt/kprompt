package intent

import "testing"

func TestNormalizeGitOpsStatus(t *testing.T) {
	got := NormalizeGitOps(Intent{Kind: KindGet}, "show gitops sync status")
	if got.Kind != KindGitOps {
		t.Fatalf("%+v", got)
	}
	action, ok := got.StringParam("action")
	if !ok || action != "status" {
		t.Fatalf("action=%v ok=%v", action, ok)
	}
	engine, ok := got.StringParam("engine")
	if !ok || engine != "auto" {
		t.Fatalf("engine=%v ok=%v", engine, ok)
	}
}

func TestNormalizeGitOpsSyncFlux(t *testing.T) {
	got := NormalizeGitOps(Intent{Kind: KindUnknown, Target: Target{Name: "apps"}}, "sync flux kustomization apps")
	if got.Kind != KindGitOps {
		t.Fatalf("%+v", got)
	}
	action, _ := got.StringParam("action")
	engine, _ := got.StringParam("engine")
	if action != "sync" || engine != "flux" {
		t.Fatalf("action=%s engine=%s", action, engine)
	}
	if got.Target.Kind != "Kustomization" {
		t.Fatalf("%+v", got)
	}
}

func TestLooksLikeGitOpsPrompt(t *testing.T) {
	if !LooksLikeGitOpsPrompt("show gitops sync status") {
		t.Fatal("expected gitops match")
	}
	if !LooksLikeGitOpsPrompt("flux kustomization health") {
		t.Fatal("expected flux match")
	}
	if !LooksLikeGitOpsPrompt("argo cd application status") {
		t.Fatal("expected argo cd match")
	}
	if LooksLikeGitOpsPrompt("submit an argo workflow") {
		t.Fatal("should not match plain argo workflow")
	}
	if LooksLikeGitOpsPrompt("rollback yesterday's deployment") {
		t.Fatal("should not match plain k8s rollback")
	}
	if LooksLikeGitOpsPrompt("scale api to 3") {
		t.Fatal("should not match scale")
	}
}

func TestLooksLikeGitOpsMutate(t *testing.T) {
	if !LooksLikeGitOpsMutate("sync argocd application payments") {
		t.Fatal("expected mutate")
	}
	if LooksLikeGitOpsMutate("show gitops sync status") {
		t.Fatal("status should be read")
	}
	if LooksLikeGitOpsMutate("list flux kustomizations") {
		t.Fatal("list should be read")
	}
}
