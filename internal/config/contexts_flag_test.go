package config

import "testing"

func TestParseContextsFlag(t *testing.T) {
	got := ParseContextsFlag(" prod, staging,prod ")
	if len(got) != 2 || got[0] != "prod" || got[1] != "staging" {
		t.Fatalf("%v", got)
	}
}

func TestResolveContextList(t *testing.T) {
	aliases := map[string]string{"prod": "gke_prod", "staging": "kind-staging"}
	got, err := ResolveContextList([]string{"prod", "staging", "gke_prod"}, aliases)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "gke_prod" || got[1] != "kind-staging" {
		t.Fatalf("%v", got)
	}
}
