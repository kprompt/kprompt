package llm

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeOpenAIBaseURL(t *testing.T) {
	got, err := RuntimeOpenAIBaseURL("us-east-1")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://bedrock-runtime.us-east-1.amazonaws.com/openai/v1"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	_, err = RuntimeOpenAIBaseURL("")
	if err == nil {
		t.Fatal("expected error for empty region")
	}
}

func TestResolveBedrockBaseURLFromRegion(t *testing.T) {
	t.Setenv("KPROMPT_OPENAI_BASE_URL", "")
	t.Setenv("KPROMPT_BEDROCK_REGION", "eu-west-1")
	got, err := ResolveBedrockBaseURL("")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://bedrock-runtime.eu-west-1.amazonaws.com/openai/v1" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveBedrockBaseURLRequiresRegionOrBase(t *testing.T) {
	t.Setenv("KPROMPT_OPENAI_BASE_URL", "")
	t.Setenv("KPROMPT_BEDROCK_REGION", "")
	t.Setenv("AWS_REGION", "")
	t.Setenv("AWS_DEFAULT_REGION", "")
	_, err := ResolveBedrockBaseURL("")
	if err == nil || !strings.Contains(err.Error(), "provider bedrock requires") {
		t.Fatalf("err = %v", err)
	}
}

func TestLookupPresetBedrock(t *testing.T) {
	p, ok := LookupPreset("bedrock")
	if !ok {
		t.Fatal("bedrock preset not found")
	}
	if p.Kind != "openai" {
		t.Fatalf("kind = %q, want openai", p.Kind)
	}
	if p.DefaultModel != "openai.gpt-oss-120b-1:0" {
		t.Fatalf("default model = %q", p.DefaultModel)
	}
	if len(p.EnvKeys) != 2 || p.EnvKeys[0] != "KPROMPT_BEDROCK_API_KEY" || p.EnvKeys[1] != "AWS_BEARER_TOKEN_BEDROCK" {
		t.Fatalf("env keys = %v", p.EnvKeys)
	}
}

func TestBedrockUsesRegionBaseURLAndBearerKey(t *testing.T) {
	t.Setenv("KPROMPT_OPENAI_BASE_URL", "")
	t.Setenv("KPROMPT_BEDROCK_REGION", "us-west-2")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "bedrock-key")

	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		if r.URL.Path != "/chat/completions" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"ok"}}]}`))
	}))
	defer srv.Close()

	t.Setenv("KPROMPT_OPENAI_BASE_URL", srv.URL)
	p, err := New("bedrock", "", "", "openai.gpt-oss-20b-1:0")
	if err != nil {
		t.Fatalf("New(bedrock) = %v", err)
	}
	resp, err := p.Complete(t.Context(), CompletionRequest{User: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Text != "ok" {
		t.Fatalf("text = %q", resp.Text)
	}
	if auth != "Bearer bedrock-key" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestBedrockMissingKeyMentionsEnv(t *testing.T) {
	t.Setenv("KPROMPT_BEDROCK_API_KEY", "")
	t.Setenv("AWS_BEARER_TOKEN_BEDROCK", "")
	t.Setenv("KPROMPT_BEDROCK_REGION", "us-east-1")
	_, err := New("bedrock", "", "", "")
	if err == nil || !strings.Contains(err.Error(), "KPROMPT_BEDROCK_API_KEY") {
		t.Fatalf("err = %v", err)
	}
}
