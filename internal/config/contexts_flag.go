package config

import (
	"fmt"
	"strings"
)

// ParseContextsFlag splits a comma-separated --contexts value.
func ParseContextsFlag(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		key := strings.ToLower(p)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, p)
	}
	return out
}

// ResolveContextList resolves aliases for each name and de-duplicates by target.
func ResolveContextList(names []string, aliases map[string]string) ([]string, error) {
	if len(names) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(names))
	seen := map[string]struct{}{}
	for _, name := range names {
		resolved, _ := ResolveContext(name, aliases)
		if resolved == "" {
			return nil, fmt.Errorf("empty context in list")
		}
		if _, ok := seen[resolved]; ok {
			continue
		}
		seen[resolved] = struct{}{}
		out = append(out, resolved)
	}
	return out, nil
}
