package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSetFieldAndBuildView(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	if _, err := SetField("provider", "gemini"); err != nil {
		t.Fatal(err)
	}
	if _, err := SetField("model", "gemini-2.0-flash"); err != nil {
		t.Fatal(err)
	}
	if _, err := SetField("namespace", "demo"); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, ".kprompt", "config.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	for _, want := range []string{"provider: gemini", "model: gemini-2.0-flash", "namespace: demo"} {
		if !strings.Contains(body, want) {
			t.Fatalf("config missing %q:\n%s", want, body)
		}
	}
	if strings.Contains(strings.ToLower(body), "api_key") || strings.Contains(body, "sk-") {
		t.Fatalf("secret leaked into file:\n%s", body)
	}

	t.Setenv("KPROMPT_GEMINI_API_KEY", "test-secret-should-not-print")
	view, err := BuildView()
	if err != nil {
		t.Fatal(err)
	}
	out := FormatView(view)
	if view.APIKey != "set" {
		t.Fatalf("api_key status=%q", view.APIKey)
	}
	if strings.Contains(out, "test-secret") {
		t.Fatalf("secret leaked into view:\n%s", out)
	}
	if !strings.Contains(out, "provider:    gemini") {
		t.Fatalf("view=%s", out)
	}
}

func TestSetFieldRejectsUnknownProvider(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := SetField("provider", "not-a-real-llm")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetFieldRejectsUnknownKey(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	_, err := SetField("password", "x")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSetFieldOTelBackend(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	file, err := SetField("tools.otel.backend", "tempo")
	if err != nil {
		t.Fatal(err)
	}
	if file.Tools.OTel.Backend != "tempo" {
		t.Fatalf("backend=%q", file.Tools.OTel.Backend)
	}
	if _, err := SetField("tools.otel.backend", "otlp"); err == nil {
		t.Fatal("expected unsupported backend error")
	}
}
