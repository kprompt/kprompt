package recipe

import (
	"strings"
	"testing"
)

func TestMatchHarden(t *testing.T) {
	r, ok := Match("please harden production in payments")
	if !ok || r.ID != "harden-production" {
		t.Fatalf("got ok=%v id=%s", ok, r.ID)
	}
}

func TestMatchBlackFriday(t *testing.T) {
	r, ok := Match("prepare for black friday")
	if !ok || r.ID != "prepare-black-friday" {
		t.Fatalf("got %+v ok=%v", r, ok)
	}
}

func TestExpandNamespace(t *testing.T) {
	r, _ := Lookup("harden-production")
	steps, err := r.Expand("payments", "")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 3 || !strings.Contains(steps[0], "payments") {
		t.Fatalf("%v", steps)
	}
}

func TestExpandWorkloadRequired(t *testing.T) {
	r, _ := Lookup("crashloop-rca")
	_, err := r.Expand("payments", "")
	if err == nil {
		t.Fatal("expected workload error")
	}
	steps, err := r.Expand("payments", "api")
	if err != nil || !strings.Contains(steps[0], "api") {
		t.Fatalf("%v %v", steps, err)
	}
}

func TestTryRoute(t *testing.T) {
	steps, r, ok, err := TryRoute("harden my cluster", "shop", "")
	if err != nil || !ok || r.ID != "harden-production" || len(steps) != 3 {
		t.Fatalf("steps=%v r=%s ok=%v err=%v", steps, r.ID, ok, err)
	}
	_, _, ok, err = TryRoute("crashloop recipe", "payments", "")
	if !ok || err == nil {
		t.Fatalf("expected workload err: ok=%v err=%v", ok, err)
	}
	steps, _, ok, err = TryRoute("crashloop recipe for api", "payments", "")
	if err != nil || !ok || len(steps) != 3 {
		t.Fatalf("steps=%v ok=%v err=%v", steps, ok, err)
	}
}

func TestCatalogStable(t *testing.T) {
	c := Catalog()
	if len(c) < 6 {
		t.Fatalf("len=%d", len(c))
	}
	if c[0].ID > c[len(c)-1].ID {
		t.Fatal("not sorted")
	}
}

func TestFormatListIncludesHeaderAndKnownRecipe(t *testing.T) {
	out := FormatList()
	if !strings.Contains(out, "ID") || !strings.Contains(out, "TITLE") || !strings.Contains(out, "STEPS") {
		t.Fatalf("missing table header in output:\n%s", out)
	}
	if !strings.Contains(out, "harden-production") {
		t.Fatalf("missing known recipe id in output:\n%s", out)
	}
}

func TestFormatShowIncludesTitleStepsAndFooter(t *testing.T) {
	r, ok := Lookup("crashloop-rca")
	if !ok {
		t.Fatal("crashloop-rca recipe not found")
	}

	out := FormatShow(r)
	if !strings.Contains(out, "Recipe: crashloop-rca") || !strings.Contains(out, r.Title) {
		t.Fatalf("missing title in output:\n%s", out)
	}
	if !strings.Contains(out, "Steps:") || !strings.Contains(out, "1. ") {
		t.Fatalf("missing steps in output:\n%s", out)
	}
	if !strings.Contains(out, "Never mutates silently") {
		t.Fatalf("missing footer in output:\n%s", out)
	}
}

func TestExtractWorkload(t *testing.T) {
	tests := []struct {
		name   string
		prompt string
		want   string
	}{
		{name: "for api", prompt: "crashloop recipe for api", want: "api"},
		{name: "workload payment-api", prompt: "run workload payment-api", want: "payment-api"},
		{name: "deployment checkout", prompt: "investigate deployment checkout", want: "checkout"},
		{name: "empty", prompt: "", want: ""},
		{name: "no match", prompt: "show service dependency graph", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractWorkload(tt.prompt)
			if got != tt.want {
				t.Fatalf("ExtractWorkload(%q) = %q, want %q", tt.prompt, got, tt.want)
			}
		})
	}
}
