package intent

import (
	"regexp"
	"strings"
)

var (
	dashboardPromptPattern = regexp.MustCompile(
		`(?i)\b(?:show|list|open|find)\b.*\bdashboards?\b`,
	)
	namedDashboardPattern = regexp.MustCompile(
		`(?i)\b(?:show|open|find)\s+(?:me\s+)?(?:the\s+)?([a-z0-9][a-z0-9-]*)\s+dashboard\b`,
	)
)

// LooksLikeDashboardPrompt reports explicit Grafana dashboard requests.
func LooksLikeDashboardPrompt(prompt string) bool {
	return dashboardPromptPattern.MatchString(prompt)
}

// NormalizeDashboard corrects generic show/list classifications.
func NormalizeDashboard(in Intent, prompt string) Intent {
	if !LooksLikeDashboardPrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindGet, KindDescribe, KindUnknown:
		in.Kind = KindDashboard
	}
	if in.Kind != KindDashboard {
		return in
	}
	if strings.TrimSpace(in.Target.Name) == "" {
		match := namedDashboardPattern.FindStringSubmatch(prompt)
		if len(match) > 1 {
			in.Target.Name = strings.ToLower(match[1])
		}
	}
	in.Target.Kind = "Dashboard"
	return in
}
