package intent

import "testing"

func TestNormalizeCrossplane(t *testing.T) {
	got := NormalizeCrossplane(Intent{Kind: KindUnknown}, "provision a postgres database")
	if got.Kind != KindCrossplane {
		t.Fatalf("%+v", got)
	}
	resource, ok := got.StringParam("resource")
	if !ok || resource != "postgres" {
		t.Fatalf("resource=%v ok=%v", resource, ok)
	}
	if got.Target.Kind != "PostgreSQLInstance" {
		t.Fatalf("%+v", got)
	}
}

func TestLooksLikeCrossplanePrompt(t *testing.T) {
	if !LooksLikeCrossplanePrompt("provision a postgres database") {
		t.Fatal("expected match")
	}
	if !LooksLikeCrossplanePrompt("create a crossplane claim for redis") {
		t.Fatal("expected match")
	}
	if LooksLikeCrossplanePrompt("scale api to 3") {
		t.Fatal("should not match")
	}
}
