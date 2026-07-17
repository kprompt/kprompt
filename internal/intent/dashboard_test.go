package intent

import "testing"

func TestNormalizeDashboardListPrompt(t *testing.T) {
	got := NormalizeVerb(Intent{Kind: KindGet}, "show dashboards")
	if got.Kind != KindDashboard {
		t.Fatalf("kind=%s", got.Kind)
	}
	if got.Target.Name != "" || got.Target.Kind != "Dashboard" {
		t.Fatalf("target=%+v", got.Target)
	}
}

func TestNormalizeNamedDashboardPrompt(t *testing.T) {
	got := NormalizeVerb(Intent{Kind: KindUnknown}, "show payments dashboard")
	if got.Kind != KindDashboard {
		t.Fatalf("kind=%s", got.Kind)
	}
	if got.Target.Name != "payments" || got.Target.Kind != "Dashboard" {
		t.Fatalf("target=%+v", got.Target)
	}
}

func TestNormalizeDashboardLeavesKubernetesShowAlone(t *testing.T) {
	got := NormalizeVerb(Intent{Kind: KindGet}, "show deployments")
	if got.Kind != KindGet {
		t.Fatalf("kind=%s", got.Kind)
	}
}
