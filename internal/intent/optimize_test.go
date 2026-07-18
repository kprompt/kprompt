package intent

import "testing"

func TestNormalizeOptimize(t *testing.T) {
	got := NormalizeVerb(Intent{Kind: KindUnknown}, "optimize my cluster")
	if got.Kind != KindOptimize {
		t.Fatalf("kind=%s", got.Kind)
	}
	got = NormalizeVerb(Intent{Kind: KindGet}, "rightsize workloads in prod")
	if got.Kind != KindOptimize {
		t.Fatalf("kind=%s", got.Kind)
	}
	got = NormalizeVerb(Intent{Kind: KindExplain}, "why is api crashing")
	if got.Kind == KindOptimize {
		t.Fatal("crash explain must not become optimize")
	}
}

func TestLooksLikeOptimizePrompt(t *testing.T) {
	if !LooksLikeOptimizePrompt("optimize my cluster") {
		t.Fatal("expected match")
	}
	if LooksLikeOptimizePrompt("list deployments") {
		t.Fatal("unexpected match")
	}
}
