package intent

import (
	"regexp"
	"strings"
)

var tracePromptPattern = regexp.MustCompile(
	`(?i)^\s*(?:trace\s+|show\s+(?:me\s+)?(?:a\s+)?trace(?:\s+for)?\s+)(?:a\s+|the\s+|my\s+)?([a-z0-9][a-z0-9-]*)`,
)

// LooksLikeTracePrompt reports prompts that request a distributed trace walk.
func LooksLikeTracePrompt(prompt string) bool {
	return tracePromptPattern.MatchString(prompt)
}

// NormalizeTrace corrects model classifications and fills safe query defaults.
func NormalizeTrace(in Intent, prompt string) Intent {
	if !LooksLikeTracePrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindExplain, KindGet, KindDescribe, KindUnknown:
		in.Kind = KindTrace
	}
	if in.Kind != KindTrace {
		return in
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	if _, ok := in.Window(); !ok {
		in.Params["window"] = "1h"
	}
	if operation, ok := in.Operation(); ok {
		switch strings.ToLower(strings.TrimSpace(operation)) {
		case "request", "requests":
			delete(in.Params, "operation")
		}
	}
	if strings.TrimSpace(in.Target.Name) == "" {
		match := tracePromptPattern.FindStringSubmatch(prompt)
		if len(match) > 1 {
			in.Target.Name = strings.ToLower(match[1])
		}
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "Service"
	}
	return in
}
