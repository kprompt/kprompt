package intent

import "testing"

func TestNormalizeGraph(t *testing.T) {
	got := NormalizeGraph(Intent{Kind: KindGet}, "show service dependency graph")
	if got.Kind != KindGraph {
		t.Fatalf("%+v", got)
	}
	got = NormalizeGraph(Intent{Kind: KindUnknown}, "service graph for my cluster")
	if got.Kind != KindGraph {
		t.Fatalf("%+v", got)
	}
	got = NormalizeGraph(Intent{Kind: KindScale}, "show service dependency graph")
	if got.Kind == KindGraph {
		t.Fatal("should not override scale")
	}
}

func TestLooksLikeGraphPrompt(t *testing.T) {
	if !LooksLikeGraphPrompt("show service dependency graph") {
		t.Fatal("expected match")
	}
	if LooksLikeGraphPrompt("list deployments") {
		t.Fatal("should not match")
	}
}

func TestApplyGraphScope(t *testing.T) {
	in := Intent{Kind: KindGraph, Target: Target{Namespace: "default"}, Params: map[string]any{"scope": "cluster"}}
	got := ApplyGraphScope(in, "show service dependency graph", ScopePrefs{})
	if got.Target.Namespace != "" {
		t.Fatalf("%+v", got)
	}
	got = ApplyGraphScope(in, "show service dependency graph in prod", ScopePrefs{})
	if got.Target.Namespace != "prod" {
		t.Fatalf("%+v", got)
	}
}
