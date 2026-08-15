package intent

import (
	"regexp"
	"strings"
)

var (
	whyCausalPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bwhy\s+is\s+.+\s+pending\b`),
		regexp.MustCompile(`(?i)\bwhy\s+is\s+.+\s+(crashlooping|crash-?looping|crashing|oom|oomkilled|image.?pull)`),
		regexp.MustCompile(`(?i)\bwhy\s+(?:is\s+)?(?:the\s+)?(?:pod|deployment)\s+.+\s+(pending|failing|broken)`),
		regexp.MustCompile(`(?i)\bcausal\s+(chain|tree|state)\b`),
	}
	// whyTargetPattern captures the workload from common why phrasings.
	whyTargetPattern = regexp.MustCompile(
		`(?i)\bwhy\s+is\s+(?:my\s+|the\s+)?(?:pod\s+|deployment\s+)?([a-z0-9][a-z0-9-]*)\b`,
	)
)

// LooksLikeWhyPrompt detects causal-state “why” questions (S-003).
// Performance (“why is X slow”) is handled separately and must not match here.
func LooksLikeWhyPrompt(prompt string) bool {
	if LooksLikePerformancePrompt(prompt) {
		return false
	}
	p := strings.ToLower(strings.TrimSpace(prompt))
	if p == "" {
		return false
	}
	for _, re := range whyCausalPatterns {
		if re.MatchString(p) {
			return true
		}
	}
	// Generic “why is <name> …” that is not slow → causal why.
	if strings.Contains(p, "why is ") || strings.HasPrefix(p, "why ") {
		if strings.Contains(p, "crash") || strings.Contains(p, "fail") ||
			strings.Contains(p, "pending") || strings.Contains(p, "oom") ||
			strings.Contains(p, "pull") || strings.Contains(p, "restart") {
			return true
		}
	}
	return false
}

// NormalizeWhy maps causal why prompts onto KindWhy.
func NormalizeWhy(in Intent, prompt string) Intent {
	if !LooksLikeWhyPrompt(prompt) {
		return in
	}
	switch in.Kind {
	// KindPerformance: bad LLM may classify crash/pending as performance; causal why wins
	// because LooksLikeWhyPrompt already excludes slow/latency prompts.
	case KindExplain, KindGet, KindDescribe, KindUnknown, KindWhy, KindInvestigate, KindPerformance:
		in.Kind = KindWhy
	default:
		return in
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		lower := strings.ToLower(prompt)
		if strings.Contains(lower, "pod ") {
			in.Target.Kind = "Pod"
		} else {
			in.Target.Kind = "Deployment"
		}
	}
	if strings.TrimSpace(in.Target.Name) == "" {
		if m := whyTargetPattern.FindStringSubmatch(prompt); len(m) == 2 {
			in.Target.Name = strings.ToLower(m[1])
		}
	}
	return in
}
