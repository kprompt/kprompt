package safety

import "testing"

func TestCheckKEDAPromptDeniesWipe(t *testing.T) {
	r := CheckKEDAPrompt("delete all scaledobjects")
	if !r.Denied {
		t.Fatal("expected deny")
	}
}

func TestCheckKEDAPromptAllowsCreate(t *testing.T) {
	r := CheckKEDAPrompt("scale api to zero with keda")
	if r.Denied {
		t.Fatal("should allow")
	}
}
