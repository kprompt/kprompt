package intent

import "testing"

func TestParseScopePhrases(t *testing.T) {
	cases := []struct {
		prompt string
		ns     string
		ctx    string
	}{
		{`list pods in staging`, "staging", ""},
		{`scale api to 3 in the prod namespace`, "prod", ""},
		{`deploy redis in production`, "production", ""},
		{`list pods in stage`, "staging", ""},
		{`show deployments on kind-kprompt-e2e context`, "", "kind-kprompt-e2e"},
		{`list pods using context docker-desktop`, "", "docker-desktop"},
		{`scale api to 2 in staging on prod-cluster context`, "staging", "prod-cluster"},
	}
	for _, c := range cases {
		ns, ctx := ParseScopePhrases(c.prompt)
		if ns != c.ns || ctx != c.ctx {
			t.Fatalf("%q => ns=%q ctx=%q want ns=%q ctx=%q", c.prompt, ns, ctx, c.ns, c.ctx)
		}
	}
}

func TestApplyScopeCLIOverrides(t *testing.T) {
	in := Intent{
		Raw:     `list pods in staging`,
		Target:  Target{Namespace: "from-llm"},
		Context: "from-llm-ctx",
	}
	got := ApplyScope(in, ScopePrefs{
		DefaultNamespace: "cli-ns",
		DefaultContext:   "cli-ctx",
		ForceNamespace:   true,
		ForceContext:     true,
	})
	if got.Target.Namespace != "cli-ns" || got.Context != "cli-ctx" {
		t.Fatalf("%+v", got)
	}
}

func TestApplyScopeHeuristicAndDefaults(t *testing.T) {
	in := Intent{Raw: `list pods in staging`}
	got := ApplyScope(in, ScopePrefs{DefaultNamespace: "default"})
	if got.Target.Namespace != "staging" {
		t.Fatalf("ns=%s", got.Target.Namespace)
	}

	in2 := Intent{Raw: `list pods`}
	got2 := ApplyScope(in2, ScopePrefs{DefaultNamespace: "ops"})
	if got2.Target.Namespace != "ops" {
		t.Fatalf("ns=%s", got2.Target.Namespace)
	}
}

func TestApplyScopeLLMBeatsHeuristic(t *testing.T) {
	in := Intent{
		Raw:    `list pods in staging`,
		Target: Target{Namespace: "production"},
	}
	got := ApplyScope(in, ScopePrefs{DefaultNamespace: "default"})
	if got.Target.Namespace != "production" {
		t.Fatalf("ns=%s", got.Target.Namespace)
	}
}
