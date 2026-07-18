package intent

import (
	"regexp"
	"strings"
)

var optimizePromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\boptimize\b.*\b(cluster|namespace|workloads?|resources?)\b`),
	regexp.MustCompile(`(?i)\b(cluster|namespace)\b.*\boptimize\b`),
	regexp.MustCompile(`(?i)\bright\s*siz(e|ing)\b`),
	regexp.MustCompile(`(?i)\b(idle|underutilized)\b.*\b(pods?|workloads?|deployments?|cluster)\b`),
}

var clusterOptimizePattern = regexp.MustCompile(`(?i)\boptimize\b.*\bcluster\b|\bcluster\b.*\boptimize\b`)

// LooksLikeOptimizePrompt reports cluster-wide optimize / rightsizing asks.
func LooksLikeOptimizePrompt(prompt string) bool {
	p := strings.TrimSpace(prompt)
	for _, re := range optimizePromptPatterns {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

// NormalizeOptimize maps optimize-shaped prompts onto kind=optimize.
func NormalizeOptimize(in Intent, prompt string) Intent {
	if !LooksLikeOptimizePrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindGet, KindExplain, KindPerformance, KindUnknown, KindOptimize:
		in.Kind = KindOptimize
	default:
		return in
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "Cluster"
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	if clusterOptimizePattern.MatchString(prompt) {
		in.Params["scope"] = "cluster"
	}
	return in
}

// ApplyOptimizeScope clears the default namespace for cluster-wide optimize prompts
// unless the CLI forced -n or the prompt named a namespace.
func ApplyOptimizeScope(in Intent, prompt string, prefs ScopePrefs) Intent {
	if in.Kind != KindOptimize {
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
