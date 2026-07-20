package safety

import "testing"

func TestCheckIstioPromptDeniesWipe(t *testing.T) {
	r := CheckIstioPrompt("delete all virtualservices")
	if !r.Denied {
		t.Fatal("expected deny")
	}
}

func TestCheckIstioPromptAllowsRead(t *testing.T) {
	r := CheckIstioPrompt("show virtualservice for payments")
	if r.Denied {
		t.Fatal("should allow")
	}
}
