package intent

import "testing"

func TestNormalizePerformanceSlowPrompt(t *testing.T) {
	in := Intent{Kind: KindExplain}
	got := NormalizeVerb(in, "why is my api slow")
	if got.Kind != KindPerformance {
		t.Fatalf("kind=%s", got.Kind)
	}
	if got.Target.Name != "api" || got.Target.Kind != "Deployment" {
		t.Fatalf("target=%+v", got.Target)
	}
	window, ok := got.Window()
	if !ok || window != "15m" {
		t.Fatalf("window=%q ok=%v", window, ok)
	}
}

func TestNormalizePerformanceLeavesCrashExplainAlone(t *testing.T) {
	in := Intent{
		Kind:   KindExplain,
		Target: Target{Name: "api"},
	}
	got := NormalizeVerb(in, "why is api crashing")
	if got.Kind != KindExplain {
		t.Fatalf("kind=%s", got.Kind)
	}
}
