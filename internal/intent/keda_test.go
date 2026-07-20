package intent

import "testing"

func TestNormalizeKEDA(t *testing.T) {
	got := NormalizeKEDA(Intent{Kind: KindScale, Target: Target{Name: "api"}}, "scale api to zero with keda")
	if got.Kind != KindKEDA {
		t.Fatalf("%+v", got)
	}
	if got.Target.Kind != "ScaledObject" {
		t.Fatalf("%+v", got)
	}
	trigger, ok := got.StringParam("trigger")
	if !ok || trigger == "" {
		t.Fatalf("trigger=%v ok=%v", trigger, ok)
	}
	min, ok := got.Params["minReplicas"]
	if !ok {
		t.Fatal("expected minReplicas")
	}
	if min != 0 && min != float64(0) && min != int(0) && min != int32(0) {
		t.Fatalf("minReplicas=%v", min)
	}
}

func TestLooksLikeKEDAPrompt(t *testing.T) {
	if !LooksLikeKEDAPrompt("create a keda scaledobject for api") {
		t.Fatal("expected match")
	}
	if !LooksLikeKEDAPrompt("event-driven scale for worker queue") {
		t.Fatal("expected event-driven match")
	}
	if LooksLikeKEDAPrompt("scale api to 0") {
		t.Fatal("plain replica scale should not match keda")
	}
	if LooksLikeKEDAPrompt("scale api to 3") {
		t.Fatal("should not match")
	}
}
