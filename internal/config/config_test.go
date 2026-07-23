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
