package intent

import (
	"regexp"
	"strings"
)

var graphPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(service\s+)?dependenc(y|ies)\b.*\bgraph\b`),
	regexp.MustCompile(`(?i)\bgraph\b.*\b(service|dependenc)`),
	regexp.MustCompile(`(?i)\bshow\b.*\bservice\s+(dependency\s+)?graph\b`),
	regexp.MustCompile(`(?i)\bservice\s+graph\b`),
}

var clusterGraphPattern = regexp.MustCompile(`(?i)\b(cluster|all\s+namespaces)\b`)

// LooksLikeGraphPrompt reports service dependency graph asks.
func LooksLikeGraphPrompt(prompt string) bool {
	p := strings.TrimSpace(prompt)
	for _, re := range graphPromptPatterns {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

// NormalizeGraph maps graph-shaped prompts onto kind=graph.
func NormalizeGraph(in Intent, prompt string) Intent {
	if !LooksLikeGraphPrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindGet, KindExplain, KindTrace, KindUnknown, KindGraph:
		in.Kind = KindGraph
	default:
		return in
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "ServiceGraph"
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	if clusterGraphPattern.MatchString(prompt) || strings.TrimSpace(in.Target.Namespace) == "" {
		// Prefer cluster scope when no namespace is named; ApplyGraphScope may clear default ns.
		if _, ok := in.Params["scope"]; !ok {
			in.Params["scope"] = "cluster"
		}
	}
	return in
}

// ApplyGraphScope clears the default namespace for cluster-wide graph prompts
// unless the CLI forced -n or the prompt named a namespace.
func ApplyGraphScope(in Intent, prompt string, prefs ScopePrefs) Intent {
	if in.Kind != KindGraph {
		return in
	}
	if prefs.ForceNamespace {
		return in
	}
	phraseNS, _ := ParseScopePhrases(prompt)
	if phraseNS != "" {
		in.Target.Namespace = phraseNS
		if in.Params != nil {
			delete(in.Params, "scope")
		}
		return in
	}
	if scope, ok := in.StringParam("scope"); ok && scope == "cluster" {
		in.Target.Namespace = ""
	}
	return in
}
