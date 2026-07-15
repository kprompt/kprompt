package intent

import (
	"regexp"
	"strings"
)

// ScopePrefs controls how namespace/context are resolved from prompt vs flags.
type ScopePrefs struct {
	DefaultNamespace string
	DefaultContext   string
	ForceNamespace   bool // true when --namespace / -n was set on the CLI
	ForceContext     bool // true when --context was set on the CLI
}

var (
	reInNamespace = regexp.MustCompile(`(?i)\bin\s+(?:the\s+)?([a-z0-9][a-z0-9-]*)\s+namespace\b`)
	reInAlias     = regexp.MustCompile(`(?i)\bin\s+(staging|stage|prod|production|dev|development|default)\b`)
	reOnContext   = regexp.MustCompile(`(?i)\b(?:on|using|with)\s+(?:the\s+)?([a-z0-9][a-z0-9@_.-]*)\s+context\b`)
	reContextOf   = regexp.MustCompile(`(?i)\bcontext\s+([a-z0-9][a-z0-9@_.-]*)\b`)
)

var namespaceAliases = map[string]string{
	"stage":       "staging",
	"staging":     "staging",
	"prod":        "prod",
	"production":  "production",
	"dev":         "dev",
	"development": "development",
	"default":     "default",
}

// ParseScopePhrases extracts namespace/context hints from natural language.
func ParseScopePhrases(prompt string) (namespace, kubeContext string) {
	p := strings.TrimSpace(prompt)
	if p == "" {
		return "", ""
	}
	if m := reInNamespace.FindStringSubmatch(p); len(m) == 2 {
		namespace = normalizeNamespace(m[1])
	} else if m := reInAlias.FindStringSubmatch(p); len(m) == 2 {
		namespace = normalizeNamespace(m[1])
	}
	if m := reOnContext.FindStringSubmatch(p); len(m) == 2 {
		kubeContext = strings.TrimSpace(m[1])
	} else if m := reContextOf.FindStringSubmatch(p); len(m) == 2 {
		// Avoid matching "context" inside unrelated phrases when already set.
		cand := strings.TrimSpace(m[1])
		if !strings.EqualFold(cand, "menu") && !strings.EqualFold(cand, "switching") {
			kubeContext = cand
		}
	}
	return namespace, kubeContext
}

func normalizeNamespace(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	if alias, ok := namespaceAliases[s]; ok {
		return alias
	}
	return s
}

// ApplyScope resolves namespace and kube context with CLI override preference.
// Priority: forced CLI flag > Intent (LLM) > phrase heuristic > defaults.
func ApplyScope(in Intent, prefs ScopePrefs) Intent {
	phraseNS, phraseCtx := ParseScopePhrases(in.Raw)

	ns := strings.TrimSpace(in.Target.Namespace)
	ctxName := strings.TrimSpace(in.Context)

	if prefs.ForceNamespace {
		ns = prefs.DefaultNamespace
	} else {
		if ns == "" {
			ns = phraseNS
		}
		if ns == "" {
			ns = prefs.DefaultNamespace
		}
		if ns == "" {
			ns = "default"
		}
	}

	if prefs.ForceContext {
		ctxName = prefs.DefaultContext
	} else {
		if ctxName == "" {
			ctxName = phraseCtx
		}
		if ctxName == "" {
			ctxName = prefs.DefaultContext
		}
	}

	in.Target.Namespace = ns
	in.Context = ctxName
	return in
}
