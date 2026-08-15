package intent

import "testing"

func TestNormalizeWhy(t *testing.T) {
	got := NormalizeVerb(Intent{Kind: KindUnknown}, "why is ledger Pending")
	if got.Kind != KindWhy || got.Target.Name != "ledger" {
		t.Fatalf("%+v", got)
	}
	got = NormalizeVerb(Intent{Kind: KindExplain}, "why is api crashing")
	if got.Kind != KindWhy {
		t.Fatalf("crash why became %s", got.Kind)
	}
	// Slow stays performance.
	got = NormalizeVerb(Intent{Kind: KindExplain}, "why is my api slow")
	if got.Kind != KindPerformance {
		t.Fatalf("slow became %s", got.Kind)
	}
	// Bad LLM said performance for a crash why — heuristic must reclaim KindWhy.
	got = NormalizeVerb(Intent{Kind: KindPerformance, Target: Target{Name: "api"}}, "why is api crashing")
	if got.Kind != KindWhy {
		t.Fatalf("performance seed crash why became %s", got.Kind)
	}
}
