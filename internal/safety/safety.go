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
	RiskLow    Risk = "low"
	RiskMedium Risk = "medium"
	RiskHigh   Risk = "high"
	RiskDenied Risk = "denied"
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
	regexp.MustCompile(`(?i)\bdelete\s+all\s+(pods|deployments|resources|namespaces|services)\b`),
	regexp.MustCompile(`(?i)\b(delete|remove|wipe)\s+(the\s+)?namespace\b`),
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
	if helmDenied := CheckHelmPrompt(p); helmDenied.Denied {
		return helmDenied
	}
	if argoDenied := CheckArgoPrompt(p); argoDenied.Denied {
		return argoDenied
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
		if a.Op != planner.OpDelete {
			continue
		}
		if strings.TrimSpace(a.Object.Name) == "" || isUnscoped(a.Object.Name) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing unscoped delete — name a single resource",
			}
		}
		if strings.EqualFold(a.Object.Kind, "Namespace") {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing namespace deletion",
			}
		}
		switch strings.ToLower(a.Object.Kind) {
		case "pod", "deployment", "service":
		case "":
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing delete without a resource kind",
			}
		default:
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: fmt.Sprintf("🛡️ Refusing delete of %s (allowed: Pod, Deployment, Service)", a.Object.Kind),
			}
		}
	}
	if helmDenied := evaluateHelmPlan(plan); helmDenied.Denied {
		return helmDenied
	}
	if argoDenied := evaluateArgoPlan(plan); argoDenied.Denied {
		return argoDenied
	}
	switch plan.Intent.Kind {
	case intent.KindDelete:
		return Result{Risk: RiskHigh, Message: "Delete requires approval"}
	case intent.KindScale, intent.KindDeploy, intent.KindInstall, intent.KindUpgrade, intent.KindRollback, intent.KindPatch, intent.KindWorkflow:
		return Result{Risk: RiskMedium, Message: "Mutation requires approval"}
	case intent.KindGet, intent.KindExplain, intent.KindLogs, intent.KindDescribe, intent.KindPerformance, intent.KindTrace, intent.KindDashboard, intent.KindOptimize, intent.KindGraph:
		// Generic Kubernetes reads and optimize reports (including Secret) are RiskLow.
		// Authorization is the caller's kubeconfig RBAC — no special Secret redaction or deny.
		// Mutating unknown kinds remains denied above; generic mutate is out of scope (T-048).
		// Optimize reports are RiskLow. Optional follow-up scale/patch plans from T-057
		// are evaluated as their own KindScale/KindPatch plans (RiskMedium) when offered.
		return Result{Risk: RiskLow}
	default:
		return Result{Risk: RiskHigh, Message: fmt.Sprintf("Unknown or unsupported intent %q", plan.Intent.Kind)}
	}
}

func isUnscoped(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "*", "all", "everything", "--all", "any":
		return true
	default:
		return false
	}
}
