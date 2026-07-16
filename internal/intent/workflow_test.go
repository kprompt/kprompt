package intent

import "testing"

func TestLooksLikeWorkflowPrompt(t *testing.T) {
	cases := []struct {
		prompt string
		want   bool
	}{
		{"train a yolov11 model", true},
		{"submit an argo workflow for training", true},
		{"deploy redis", false},
		{"scale api to 3", false},
	}
	for _, tc := range cases {
		if got := LooksLikeWorkflowPrompt(tc.prompt); got != tc.want {
			t.Fatalf("prompt=%q got=%v want=%v", tc.prompt, got, tc.want)
		}
	}
}
