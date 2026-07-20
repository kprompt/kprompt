package intent

import "testing"

func TestNormalizeIstio(t *testing.T) {
	got := NormalizeIstio(Intent{Kind: KindGet}, "show virtualservice for payments")
	if got.Kind != KindIstio {
		t.Fatalf("%+v", got)
	}
	if got.Target.Kind != "VirtualService" {
		t.Fatalf("%+v", got)
	}
}

func TestLooksLikeIstioPrompt(t *testing.T) {
	if !LooksLikeIstioPrompt("show istio canary traffic for api") {
		t.Fatal("expected match")
	}
	if !LooksLikeIstioPrompt("traffic split for payments") {
		t.Fatal("expected traffic split match")
	}
	if LooksLikeIstioPrompt("scale api to 3") {
		t.Fatal("should not match")
	}
}
