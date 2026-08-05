package onboard

import (
	"bytes"
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/config"
)

func TestRunOllamaNonInteractive(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	var out bytes.Buffer
	falseVal := false
	res, err := Run(context.Background(), Options{
		Ollama:      true,
		Interactive: &falseVal,
		Out:         &out,
		HTTPClient:  &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, context.DeadlineExceeded
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.Wrote || res.Provider != "ollama" {
		t.Fatalf("res=%+v", res)
	}
	if res.Model == "" {
		t.Fatal("expected default model")
	}
	f, err := config.LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if f.Provider != "ollama" {
		t.Fatalf("file provider=%q", f.Provider)
	}
	if !strings.Contains(out.String(), "Saved") {
		t.Fatalf("out=%s", out.String())
	}
}

func TestRunNonInteractiveRequiresFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	falseVal := false
	_, err := Run(context.Background(), Options{
		Interactive: &falseVal,
		Out:         &bytes.Buffer{},
	})
	if err == nil || !strings.Contains(err.Error(), "--ollama") {
		t.Fatalf("err=%v", err)
	}
}

func TestRunDryRun(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	falseVal := false
	var out bytes.Buffer
	res, err := Run(context.Background(), Options{
		Provider:    "openai",
		DryRun:      true,
		Interactive: &falseVal,
		Out:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Wrote {
		t.Fatal("dry-run should not write")
	}
	f, err := config.LoadFile()
	if err != nil {
		t.Fatal(err)
	}
	if f.Provider != "" {
		t.Fatalf("should not persist: %+v", f)
	}
	if !strings.Contains(out.String(), "Dry-run") {
		t.Fatalf("out=%s", out.String())
	}
}

func TestRunOpenAIPrintsExportHint(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", filepath.Join(dir, ".kprompt"))

	falseVal := false
	var out bytes.Buffer
	_, err := Run(context.Background(), Options{
		Provider:    "openai",
		Interactive: &falseVal,
		Out:         &out,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "KPROMPT_OPENAI_API_KEY") {
		t.Fatalf("out=%s", out.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
