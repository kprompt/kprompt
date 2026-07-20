package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

var kedaWipePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(delete|remove|wipe)\b.*\b(all|every)\b.*\b(scaledobject|keda)`),
	regexp.MustCompile(`(?i)\b(scaledobject|keda)s?\b.*\b(delete|remove|wipe)\b.*\b(all|every)\b`),
}

// CheckKEDAPrompt denies wipe-class KEDA prompts.
func CheckKEDAPrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range kedaWipePatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing wipe-class KEDA delete — name a single ScaledObject",
			}
		}
	}
	return Result{Risk: RiskLow}
}

func evaluateKEDAPlan(plan planner.ExecutionPlan) Result {
	for _, a := range plan.Actions {
		if a.Backend != "keda" {
			continue
		}
		if a.Op == planner.OpScaledObjectCreate {
			continue
		}
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: fmt.Sprintf("🛡️ Refusing unsupported KEDA action %q", a.Op),
		}
	}
	return Result{Risk: RiskLow}
}
