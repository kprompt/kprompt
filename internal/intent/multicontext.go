package intent

import (
	"regexp"
	"strings"
)

var (
	reAcross = regexp.MustCompile(`(?i)\bacross\s+(.+?)(?:\s+contexts?)?\s*[.!]?\s*$`)
	reOnAnd  = regexp.MustCompile(`(?i)\b(?:on|using|with)\s+([a-z0-9][a-z0-9@_.-]*)\s+and\s+([a-z0-9][a-z0-9@_.-]*)\b`)
)

// ParseMultiContexts extracts an explicit multi-context list from NL.
// Examples: "across staging and prod", "across a, b, and c", "on staging and prod".
func ParseMultiContexts(prompt string) []string {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return nil
	}
	if m := reAcross.FindStringSubmatch(p); len(m) == 2 {
		return splitContextNames(m[1])
	}
	if m := reOnAnd.FindStringSubmatch(p); len(m) == 3 {
		return uniqueContextNames([]string{m[1], m[2]})
	}
	return nil
}

func splitContextNames(raw string) []string {
	raw = strings.TrimSpace(raw)
	raw = strings.ReplaceAll(raw, " and ", ",")
	raw = strings.ReplaceAll(raw, " And ", ",")
	parts := strings.Split(raw, ",")
	names := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		p = strings.Trim(p, ".")
		if p == "" || strings.EqualFold(p, "contexts") || strings.EqualFold(p, "context") {
			continue
		}
		// Drop trailing "contexts" word fragments.
		fields := strings.Fields(p)
		if len(fields) == 0 {
			continue
		}
		names = append(names, fields[0])
	}
	return uniqueContextNames(names)
}

func uniqueContextNames(names []string) []string {
	out := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" {
			continue
		}
		key := strings.ToLower(n)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, n)
	}
	if len(out) < 2 {
		return nil
	}
	return out
}
