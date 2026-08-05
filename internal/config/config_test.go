package config

import "testing"

func TestMergeResolvesContextAlias(t *testing.T) {
	file := File{
		Aliases: map[string]string{"prod": "gke_prod"},
		Context: "prod",
	}
	resolved := Merge(file, "", "", "", "", false, "list pods")
	if resolved.Context != "gke_prod" {
		t.Fatalf("context=%q", resolved.Context)
	}
	if resolved.ContextAlias != "prod" {
		t.Fatalf("alias=%q", resolved.ContextAlias)
	}
	cli := Merge(file, "", "", "staging", "", false, "x")
	// staging not in aliases → raw
	if cli.Context != "staging" {
		t.Fatalf("cli context=%q", cli.Context)
	}
}

func TestMergeEmptyProviderUnconfigured(t *testing.T) {
	r := Merge(File{}, "", "", "", "", false, "list pods")
	if r.Provider != "" {
		t.Fatalf("want empty provider, got %q", r.Provider)
	}
	if r.Model != "" {
		t.Fatalf("want empty model when unconfigured, got %q", r.Model)
	}
}

func TestMergeExplicitOpenAI(t *testing.T) {
	r := Merge(File{Provider: "openai"}, "", "", "", "", false, "x")
	if r.Provider != "openai" {
		t.Fatalf("provider=%q", r.Provider)
	}
	if r.Model == "" {
		t.Fatal("expected openai default model")
	}
}

func TestMergeCLIProviderOverride(t *testing.T) {
	r := Merge(File{}, "ollama", "", "", "", false, "x")
	if r.Provider != "ollama" {
		t.Fatalf("provider=%q", r.Provider)
	}
}
