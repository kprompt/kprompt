package safety

import "testing"

func TestCheckArgoPromptDeniesWipeAllWorkflows(t *testing.T) {
	r := CheckArgoPrompt("delete all workflows in argo")
	if !r.Denied {
		t.Fatal("expected deny")
	}
}

func TestCheckArgoPromptAllowsTrain(t *testing.T) {
	r := CheckArgoPrompt("train a yolov11 model")
	if r.Denied {
		t.Fatal("expected allow")
	}
}
