package llm

import (
	"strings"
	"testing"
)

func TestNewEmptyProvider(t *testing.T) {
	_, err := New("", "", "", "")
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "init --ollama") {
		t.Fatalf("err=%v", err)
	}
}

func TestMissingKeyErrorMentionsInit(t *testing.T) {
	_, err := New("openai", "", "", "gpt-4o-mini")
	if err == nil {
		t.Fatal("expected missing key")
	}
	msg := err.Error()
	if !strings.Contains(msg, "KPROMPT_OPENAI_API_KEY") {
		t.Fatalf("err=%v", err)
	}
	if !strings.Contains(msg, "init --ollama") {
		t.Fatalf("err=%v", err)
	}
}
