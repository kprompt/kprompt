package safety

import "testing"

func TestCheckIstioPromptAllowsRead(t *testing.T) {
	r := CheckIstioPrompt("show virtualservice for payments")
	if r.Denied {
		t.Fatal("should allow")
	}
}
