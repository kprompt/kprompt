package safety

import "testing"

func TestCheckTektonPromptDeniesWipe(t *testing.T) {
	r := CheckTektonPrompt("delete all pipelineruns")
	if !r.Denied {
		t.Fatal("expected deny")
	}
}

func TestCheckTektonPromptAllowsCI(t *testing.T) {
	r := CheckTektonPrompt("create a CI pipeline")
	if r.Denied {
		t.Fatal("should allow")
	}
}
