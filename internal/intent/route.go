package intent

import (
	"regexp"
	"strings"
)

const MaxRouteSteps = 8

var (
	explicitRouteSeparator = regexp.MustCompile(
		`(?i)\s*(?:;|,?\s*\band\s+then\b|,?\s*\bthen\b)\s*`,
	)
	andRouteSeparator = regexp.MustCompile(`(?i)\s+\band\b\s+`)
	routableClause    = regexp.MustCompile(
		`(?i)\b(?:deploy|install|upgrade|scale|rollback|undo|list|show|get|explain|logs?|describe|train|workflow|slow|latency|performance|trace|dashboards?|delete|remove)\b`,
	)
)

// SplitRoutePrompt returns sequential tool requests from an explicit NL chain.
// A plain "and" is split only when both sides independently look routable.
func SplitRoutePrompt(prompt string) []string {
	prompt = strings.TrimSpace(prompt)
	if prompt == "" {
		return nil
	}
	if parts := cleanRouteParts(explicitRouteSeparator.Split(prompt, -1)); len(parts) > 1 {
		return parts
	}
	parts := cleanRouteParts(andRouteSeparator.Split(prompt, -1))
	if len(parts) > 1 {
		for _, part := range parts {
			if !routableClause.MatchString(part) {
				return []string{prompt}
			}
		}
		return parts
	}
	return []string{prompt}
}

func cleanRouteParts(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
