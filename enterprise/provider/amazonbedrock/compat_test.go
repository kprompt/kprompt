package amazonbedrock

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/llm"
)

func TestRuntimeOpenAIBaseURLDelegates(t *testing.T) {
	got, err := RuntimeOpenAIBaseURL("ap-southeast-2")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://bedrock-runtime.ap-southeast-2.amazonaws.com/openai/v1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestNewCompatChatCompletions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"pong"}}]}`))
	}))
	defer srv.Close()

	p, err := NewCompat("key", srv.URL, "", "openai.gpt-oss-20b-1:0")
	if err != nil {
		t.Fatal(err)
	}
	out, err := p.Complete(t.Context(), llm.CompletionRequest{User: "ping"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Text != "pong" {
		t.Fatalf("text = %q", out.Text)
	}
}

func TestNewCompatFromRegion(t *testing.T) {
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "k")
	t.Setenv("KPROMPT_OPENAI_BASE_URL", "")
	_, err := NewCompat("", "", "us-east-1", "")
	if err != nil {
		t.Fatal(err)
	}
}

func TestNewCompatRequiresBaseOrRegion(t *testing.T) {
	t.Setenv("KPROMPT_BEDROCK_API_KEY", "k")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "")
	_, err := NewCompat("k", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "provider bedrock requires") {
		t.Fatalf("err = %v", err)
	}
}
