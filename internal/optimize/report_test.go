package optimize

import (
	"strings"
	"testing"
)

func TestBuildScaffoldCluster(t *testing.T) {
	rep := BuildScaffold(Request{})
	if rep.Type != "optimize" || rep.Scope != ScopeCluster {
		t.Fatalf("%+v", rep)
	}
	if rep.Summary == "" || len(rep.Findings) == 0 {
		t.Fatal("expected summary and findings")
	}
	if rep.Sections.Inventory.Status != SectionPending {
		t.Fatalf("inventory=%+v", rep.Sections.Inventory)
	}
	if len(rep.Suggestions) != 0 {
		t.Fatal("scaffold must not invent suggestions")
	}
}

func TestBuildScaffoldNamespace(t *testing.T) {
	rep := BuildScaffold(Request{Namespace: "prod"})
	if rep.Scope != ScopeNamespace || rep.Namespace != "prod" {
		t.Fatalf("%+v", rep)
	}
	if !strings.Contains(rep.Summary, "prod") {
		t.Fatalf("summary=%s", rep.Summary)
	}
}
