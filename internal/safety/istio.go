package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

var istioWipePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(delete|remove|wipe)\b.*\b(all|every)\b.*\b(virtualservice|istio)`),
	regexp.MustCompile(`(?i)\b(virtualservice|istio)\b.*\b(delete|remove|wipe)\b.*\b(all|every)\b`),
}

// CheckIstioPrompt denies wipe-class Istio prompts.
func CheckIstioPrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range istioWipePatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing wipe-class Istio delete — name a single VirtualService",
			}
		}
	}
	return Result{Risk: RiskLow}
}

func evaluateIstioPlan(plan planner.ExecutionPlan) Result {
	for _, a := range plan.Actions {
		if a.Backend != "istio" {
			continue
		}
		if a.Op == planner.OpIstioTraffic {
			continue
		}
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: fmt.Sprintf("🛡️ Refusing unsupported Istio action %q (read-first: traffic summary only)", a.Op),
		}
	}
	return Result{Risk: RiskLow}
}
