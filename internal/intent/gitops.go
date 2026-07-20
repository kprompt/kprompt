package intent

import (
	"regexp"
	"strings"
)

var gitopsPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bflux\b`),
	regexp.MustCompile(`(?i)\bargo\s*cd\b`),
	regexp.MustCompile(`(?i)\bargocd\b`),
	regexp.MustCompile(`(?i)\bgitops\b`),
	regexp.MustCompile(`(?i)\bkustomization\b`),
	regexp.MustCompile(`(?i)\b(sync|promote)\b.*\b(application|kustomization|flux|argocd|gitops)\b`),
	regexp.MustCompile(`(?i)\b(application|kustomization|flux|argocd|gitops)\b.*\b(sync|promote|health|status)\b`),
	regexp.MustCompile(`(?i)\bshow\b.*\b(gitops|sync\s+status)\b`),
}

var gitopsMutatePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(sync|reconcile|promote|rollback)\b`),
}

var plainArgoWorkflow = regexp.MustCompile(`(?i)\bargo\s+workflow`)
var plainK8sRollback = regexp.MustCompile(`(?i)\brollback\b.*\b(deployment|yesterday)`)

// LooksLikeGitOpsPrompt reports natural-language that needs Flux/Argo CD (T-043).
// Plain Argo Workflow and plain K8s "rollback yesterday's deployment" are excluded.
func LooksLikeGitOpsPrompt(prompt string) bool {
	p := strings.TrimSpace(prompt)
	if plainArgoWorkflow.MatchString(p) && !regexp.MustCompile(`(?i)\b(gitops|argocd|argo\s*cd|flux|kustomization)\b`).MatchString(p) {
		return false
	}
	if plainK8sRollback.MatchString(p) && !regexp.MustCompile(`(?i)\b(gitops|argocd|argo\s*cd|flux|kustomization|application)\b`).MatchString(p) {
		return false
	}
	for _, re := range gitopsPromptPatterns {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}

// LooksLikeGitOpsMutate reports sync/promote/rollback (status/health/show/list stay read).
func LooksLikeGitOpsMutate(prompt string) bool {
	if !LooksLikeGitOpsPrompt(prompt) {
		return false
	}
	p := strings.ToLower(prompt)
	if strings.Contains(p, "status") || strings.Contains(p, "health") ||
		strings.Contains(p, "show") || strings.Contains(p, "list") || strings.Contains(p, "get ") {
		// Explicit read verbs win unless paired with a mutate verb as the primary ask.
		if !gitopsMutatePatterns[0].MatchString(p) {
			return false
		}
		// "show sync status" is read; "sync application" is mutate.
		if regexp.MustCompile(`(?i)\b(show|list|get|status|health)\b`).MatchString(p) &&
			!regexp.MustCompile(`(?i)\b(sync|reconcile|promote|rollback)\b.*\b(application|kustomization|flux|argocd)\b`).MatchString(p) &&
			!regexp.MustCompile(`(?i)\b(sync|reconcile|promote|rollback)\s+(the\s+)?(app|application|kustomization)\b`).MatchString(p) {
			return false
		}
	}
	for _, re := range gitopsMutatePatterns {
		if re.MatchString(prompt) {
			return true
		}
	}
	return false
}

// NormalizeGitOps maps Flux/Argo CD prompts onto kind=gitops.
func NormalizeGitOps(in Intent, prompt string) Intent {
	if !LooksLikeGitOpsPrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindGet, KindDescribe, KindExplain, KindRollback, KindUnknown, KindGitOps, KindDeploy:
		in.Kind = KindGitOps
	default:
		if in.Kind != KindGitOps {
			return in
		}
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	p := strings.ToLower(prompt)

	if _, ok := in.StringParam("action"); !ok {
		switch {
		case strings.Contains(p, "promote"):
			in.Params["action"] = "promote"
		case strings.Contains(p, "rollback"):
			in.Params["action"] = "rollback"
		case LooksLikeGitOpsMutate(prompt) && (strings.Contains(p, "sync") || strings.Contains(p, "reconcile")):
			in.Params["action"] = "sync"
		default:
			in.Params["action"] = "status"
		}
	}

	if _, ok := in.StringParam("engine"); !ok {
		switch {
		case strings.Contains(p, "flux") || strings.Contains(p, "kustomization"):
			in.Params["engine"] = "flux"
		case strings.Contains(p, "argocd") || strings.Contains(p, "argo cd") || strings.Contains(p, "argo-cd"):
			in.Params["engine"] = "argocd"
		default:
			in.Params["engine"] = "auto"
		}
	}

	action, _ := in.StringParam("action")
	engine, _ := in.StringParam("engine")
	if strings.TrimSpace(in.Target.Kind) == "" {
		switch {
		case engine == "flux" || strings.EqualFold(in.Target.Kind, "Kustomization"):
			in.Target.Kind = "Kustomization"
		case engine == "argocd":
			in.Target.Kind = "Application"
		case action != "status":
			in.Target.Kind = "Application"
		default:
			in.Target.Kind = "Application"
		}
	}
	return in
}
