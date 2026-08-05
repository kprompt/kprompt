package safety

import "testing"

func TestCheckTektonPromptAllowsCI(t *testing.T) {
	r := CheckTektonPrompt("create a CI pipeline")
	if r.Denied {
		t.Fatal("should allow")
	}
}
