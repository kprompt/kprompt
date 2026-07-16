package intent

import (
	"regexp"
	"strings"
)

var (
	performancePromptPatterns = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bwhy\s+is\s+.+\s+slow\b`),
		regexp.MustCompile(`(?i)\b(slow|latency|performance|bottleneck)\b`),
		regexp.MustCompile(`(?i)\b(high|spiking)\s+(cpu|memory|latency)\b`),
	}
	slowTargetPattern = regexp.MustCompile(`(?i)\bwhy\s+is\s+(?:my\s+|the\s+)?([a-z0-9][a-z0-9-]*)\s+slow\b`)
)

// LooksLikePerformancePrompt reports prompts that need metrics-backed diagnosis.
func LooksLikePerformancePrompt(prompt string) bool {
	for _, pattern := range performancePromptPatterns {
		if pattern.MatchString(prompt) {
			return true
		}
	}
	return false
}

// NormalizePerformance corrects explain/unknown classifications for slow prompts.
func NormalizePerformance(in Intent, prompt string) Intent {
	if !LooksLikePerformancePrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindExplain, KindDescribe, KindUnknown:
		in.Kind = KindPerformance
	}
	if in.Kind != KindPerformance {
		return in
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	if _, ok := in.StringParam("window"); !ok {
		in.Params["window"] = "15m"
	}
	if strings.TrimSpace(in.Target.Name) == "" {
		match := slowTargetPattern.FindStringSubmatch(prompt)
		if len(match) > 1 {
			in.Target.Name = strings.ToLower(match[1])
		}
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "Deployment"
	}
	return in
}
