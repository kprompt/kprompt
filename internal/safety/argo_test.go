package safety

import "testing"

func TestCheckArgoPromptAllowsTrain(t *testing.T) {
	r := CheckArgoPrompt("train a yolov11 model")
	if r.Denied {
		t.Fatal("expected allow")
	}
}
