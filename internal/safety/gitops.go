package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

var gitopsWipePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(delete|remove|wipe)\b.*\b(all|every)\b.*\b(application|kustomization|gitops|flux|argocd)`),
	regexp.MustCompile(`(?i)\b(application|kustomization|gitops|flux|argocd)\b.*\b(delete|remove|wipe)\b.*\b(all|every)\b`),
}

// CheckGitOpsPrompt denies wipe-class GitOps prompts.
func CheckGitOpsPrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range gitopsWipePatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing wipe-class GitOps delete — name a single Application or Kustomization",
			}
		}
	}
	return Result{Risk: RiskLow}
}

func evaluateGitOpsPlan(plan planner.ExecutionPlan) Result {
	for _, a := range plan.Actions {
		if a.Backend != "gitops" {
			continue
		}
		switch a.Op {
		case planner.OpGitOpsStatus, planner.OpGitOpsSync:
			continue
		default:
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: fmt.Sprintf("🛡️ Refusing unsupported GitOps action %q", a.Op),
			}
		}
	}
	return Result{Risk: RiskLow}
}
