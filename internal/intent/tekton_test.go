package intent

import (
	"strings"
	"testing"
)

func TestNormalizeTekton(t *testing.T) {
	got := NormalizeTekton(Intent{Kind: KindUnknown}, "create a CI pipeline for https://github.com/acme/app")
	if got.Kind != KindTekton {
		t.Fatalf("%+v", got)
	}
	if got.Target.Kind != "PipelineRun" {
		t.Fatalf("%+v", got)
	}
	url, ok := got.RepoURL()
	if !ok || !strings.Contains(url, "github.com/acme/app") {
		t.Fatalf("repo=%v ok=%v", url, ok)
	}
}

func TestLooksLikeTektonPrompt(t *testing.T) {
	if !LooksLikeTektonPrompt("create a CI pipeline") {
		t.Fatal("expected match")
	}
	if LooksLikeTektonPrompt("scale api to 3") {
		t.Fatal("should not match")
	}
}
