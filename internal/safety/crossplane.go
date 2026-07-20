package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

var crossplaneWipePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(delete|remove|wipe)\b.*\b(all|every)\b.*\b(claim|crossplane|cloud\s+resource)`),
	regexp.MustCompile(`(?i)\b(claim|crossplane)\b.*\b(delete|remove|wipe)\b.*\b(all|every)\b`),
}

// CheckCrossplanePrompt denies wipe-class Crossplane prompts.
func CheckCrossplanePrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range crossplaneWipePatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing wipe-class Crossplane delete — name a single claim",
			}
		}
	}
	return Result{Risk: RiskLow}
}

func evaluateCrossplanePlan(plan planner.ExecutionPlan) Result {
	for _, a := range plan.Actions {
		if a.Backend != "crossplane" {
			continue
		}
		if a.Op == planner.OpClaimCreate {
			continue
		}
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: fmt.Sprintf("🛡️ Refusing unsupported Crossplane action %q", a.Op),
		}
	}
	return Result{Risk: RiskLow}
}
