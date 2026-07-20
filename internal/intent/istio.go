package intent

import (
	"regexp"
	"strings"
)

var istioPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bistio\b`),
	regexp.MustCompile(`(?i)\bvirtual\s*service\b`),
	regexp.MustCompile(`(?i)\bvirtualservice\b`),
	regexp.MustCompile(`(?i)\b(canary|traffic\s+split|traffic\s+shift)\b`),
	regexp.MustCompile(`(?i)\b(mesh|envoy)\b.*\b(route|traffic|virtual)\b`),
	regexp.MustCompile(`(?i)\bshow\b.*\b(traffic|routes?)\b.*\b(for|of)\b`),
}

// LooksLikeIstioPrompt reports natural-language that needs Istio VirtualService reads (T-041).
func LooksLikeIstioPrompt(prompt string) bool {
	for _, re := range istioPromptPatterns {
		if re.MatchString(prompt) {
			return true
		}
	}
	return false
}

// NormalizeIstio maps mesh/traffic prompts onto kind=istio (read-first).
func NormalizeIstio(in Intent, prompt string) Intent {
	if !LooksLikeIstioPrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindGet, KindDescribe, KindExplain, KindUnknown, KindIstio:
		in.Kind = KindIstio
	default:
		if in.Kind != KindIstio {
			return in
		}
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "VirtualService"
	}
	return in
}
