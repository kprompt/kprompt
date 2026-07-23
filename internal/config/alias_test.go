package config

import "testing"

func TestResolveContext(t *testing.T) {
	aliases := map[string]string{"prod": "gke_prod", "staging": "kind-staging"}
	got, used := ResolveContext("prod", aliases)
	if got != "gke_prod" || used != "prod" {
		t.Fatalf("got=%q used=%q", got, used)
	}
	got, used = ResolveContext("PROD", aliases)
	if got != "gke_prod" || used != "prod" {
		t.Fatalf("case: got=%q used=%q", got, used)
	}
	got, used = ResolveContext("kind-dev", aliases)
	if got != "kind-dev" || used != "" {
		t.Fatalf("raw: got=%q used=%q", got, used)
	}
	got, used = ResolveContext("", aliases)
	if got != "" || used != "" {
		t.Fatalf("empty: got=%q used=%q", got, used)
	}
}

func TestSetUnsetAlias(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	f, err := SetAlias("prod", "gke_prod")
	if err != nil {
		t.Fatal(err)
	}
	if f.Aliases["prod"] != "gke_prod" {
		t.Fatalf("%v", f.Aliases)
	}
	f, err = UnsetAlias("prod")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Aliases) != 0 {
		t.Fatalf("%v", f.Aliases)
	}
	if _, err := UnsetAlias("missing"); err == nil {
		t.Fatal("expected error")
	}
}

func TestAliasLines(t *testing.T) {
	lines := AliasLines(map[string]string{"z": "c2", "a": "c1"})
	if len(lines) != 2 || lines[0] != "a → c1" || lines[1] != "z → c2" {
		t.Fatalf("%v", lines)
	}
}
