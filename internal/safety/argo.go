package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

var argoWipePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(delete|remove|wipe)\b.*\b(all|every)\b.*\bworkflow`),
	regexp.MustCompile(`(?i)\bworkflow(s)?\b.*\b(delete|remove|wipe)\b.*\b(all|every)\b`),
}

// CheckArgoPrompt denies wipe-class Argo workflow prompts.
func CheckArgoPrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range argoWipePatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing wipe-class Argo workflow delete — name a single workflow",
			}
		}
	}
	return Result{Risk: RiskLow}
}

func evaluateArgoPlan(plan planner.ExecutionPlan) Result {
	for _, a := range plan.Actions {
		if a.Backend != "argo" {
			continue
		}
		if a.Op == planner.OpWorkflowCreate {
			continue
		}
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: fmt.Sprintf("🛡️ Refusing unsupported Argo action %q", a.Op),
		}
	}
	return Result{Risk: RiskLow}
}
