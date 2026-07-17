package intent

import (
	"reflect"
	"testing"
)

func TestSplitRoutePromptExplicitSequence(t *testing.T) {
	got := SplitRoutePrompt(
		"why is api slow, then trace payment request; show payments dashboard",
	)
	want := []string{
		"why is api slow",
		"trace payment request",
		"show payments dashboard",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestSplitRoutePromptRoutableAnd(t *testing.T) {
	got := SplitRoutePrompt("why is api slow and trace payment request")
	want := []string{"why is api slow", "trace payment request"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got=%q want=%q", got, want)
	}
}

func TestSplitRoutePromptKeepsSingleResourceList(t *testing.T) {
	prompt := "show pods and services"
	got := SplitRoutePrompt(prompt)
	if !reflect.DeepEqual(got, []string{prompt}) {
		t.Fatalf("got=%q", got)
	}
}
