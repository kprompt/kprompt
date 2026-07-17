package intent

import "testing"

func TestNormalizeTracePrompt(t *testing.T) {
	got := NormalizeVerb(Intent{
		Kind:   KindExplain,
		Params: map[string]any{"operation": "request"},
	}, "trace payment request")
	if got.Kind != KindTrace {
		t.Fatalf("kind=%s", got.Kind)
	}
	if got.Target.Name != "payment" || got.Target.Kind != "Service" {
		t.Fatalf("target=%+v", got.Target)
	}
	window, ok := got.Window()
	if !ok || window != "1h" {
		t.Fatalf("window=%q ok=%v", window, ok)
	}
	if operation, ok := got.Operation(); ok {
		t.Fatalf("generic operation should not filter traces: %q", operation)
	}
}

func TestNormalizeShowTracePrompt(t *testing.T) {
	got := NormalizeVerb(Intent{Kind: KindGet}, "show me a trace for checkout")
	if got.Kind != KindTrace || got.Target.Name != "checkout" {
		t.Fatalf("intent=%+v", got)
	}
}

func TestNormalizeTraceLeavesNonTracePromptAlone(t *testing.T) {
	got := NormalizeVerb(Intent{Kind: KindExplain}, "why is payment failing")
	if got.Kind != KindExplain {
		t.Fatalf("kind=%s", got.Kind)
	}
}
