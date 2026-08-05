package safety

import "testing"

func TestCheckKEDAPromptAllowsCreate(t *testing.T) {
	r := CheckKEDAPrompt("scale api to zero with keda")
	if r.Denied {
		t.Fatal("should allow")
	}
}
