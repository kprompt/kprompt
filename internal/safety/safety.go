package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

// Risk levels for an execution plan.
type Risk string

const (
	RiskLow     Risk = "low"
	RiskMedium  Risk = "medium"
	RiskHigh    Risk = "high"
	RiskDenied  Risk = "denied"
)

// Result is the outcome of a safety evaluation.
type Result struct {
	Risk    Risk
	Denied  bool
	Message string
}

var hardDenyPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(delete|remove|wipe|destroy|drop)\b.*\b(cluster|everything|all\s+namespaces)\b`),
	regexp.MustCompile(`(?i)\b(cluster)\b.*\b(delete|remove|wipe|destroy)\b`),
	regexp.MustCompile(`(?i)\bwipe\s+(the\s+)?cluster\b`),
	regexp.MustCompile(`(?i)\bdelete\s+all\s+(pods|deployments|resources|namespaces)\b`),
	regexp.MustCompile(`(?i)\bf\*?cking\s+cluster\b`),
}

// CheckPrompt hard-denies destructive natural-language prompts before LLM spend.
func CheckPrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range hardDenyPatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🚨 Intent: destructive cluster operation\n🛡️ Safe execution: denied\n😅 Your cluster lives another day",
			}
		}
	}
	return Result{Risk: RiskLow}
}

// EvaluatePlan scores a plan and may deny unsafe actions.
func EvaluatePlan(plan planner.ExecutionPlan) Result {
	if plan.Intent.Kind == intent.KindDeny {
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: "🛡️ Intent classified as deny — aborting",
		}
	}
	for _, a := range plan.Actions {
		if a.Op == planner.OpDelete && strings.EqualFold(a.Object.Kind, "Namespace") && a.Object.Name == "" {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing unscoped namespace deletion",
			}
		}
	}
	switch plan.Intent.Kind {
	case intent.KindScale, intent.KindDeploy:
		return Result{Risk: RiskMedium, Message: "Mutation requires approval"}
	case intent.KindGet, intent.KindExplain:
		return Result{Risk: RiskLow}
	default:
		return Result{Risk: RiskHigh, Message: fmt.Sprintf("Unknown or unsupported intent %q", plan.Intent.Kind)}
	}
}
