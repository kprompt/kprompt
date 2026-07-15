package safety

import "testing"

func TestCheckPromptDeniesWipe(t *testing.T) {
	cases := []string{
		`remove my f*cking cluster`,
		`delete the cluster`,
		`wipe the cluster now`,
		`delete all namespaces`,
		`destroy everything in the cluster`,
	}
	for _, c := range cases {
		r := CheckPrompt(c)
		if !r.Denied {
			t.Fatalf("expected deny for %q", c)
		}
		if r.Risk != RiskDenied {
			t.Fatalf("expected risk denied for %q, got %s", c, r.Risk)
		}
	}
}

func TestCheckPromptAllowsScale(t *testing.T) {
	r := CheckPrompt(`scale api to 10`)
	if r.Denied {
		t.Fatal("scale should not be denied")
	}
}
