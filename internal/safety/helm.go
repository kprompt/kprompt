package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

var helmWipePromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bhelm\s+uninstall\b.*\b(--all|\ball\b)\b`),
	regexp.MustCompile(`(?i)\buninstall\s+all\s+(helm\s+)?releases?\b`),
	regexp.MustCompile(`(?i)\bpurge\s+all\s+(helm\s+)?releases?\b`),
	regexp.MustCompile(`(?i)\bhelm\s+delete\b.*\b(--all|\ball\b)\b`),
}

// CheckHelmPrompt denies wipe-class Helm uninstall prompts.
func CheckHelmPrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range helmWipePromptPatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing wipe-class Helm uninstall — name a single release",
			}
		}
	}
	return Result{Risk: RiskLow}
}

func evaluateHelmPlan(plan planner.ExecutionPlan) Result {
	for _, a := range plan.Actions {
		if a.Backend != "helm" {
			continue
		}
		if denied := evaluateHelmCommand(strings.Join(a.Command, " ")); denied.Denied {
			return denied
		}
	}
	return Result{}
}

func evaluateHelmCommand(cmd string) Result {
	lower := strings.ToLower(strings.TrimSpace(cmd))
	if !strings.Contains(lower, "uninstall") && !strings.Contains(lower, "delete") {
		return Result{}
	}
	if strings.Contains(lower, "--all") ||
		strings.Contains(lower, " all releases") ||
		strings.Contains(lower, "all helm") {
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: "🛡️ Refusing Helm uninstall without a narrow release target",
		}
	}
	if strings.Contains(lower, "uninstall") && strings.Contains(lower, " --all") {
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: fmt.Sprintf("🛡️ Refusing dangerous Helm command"),
		}
	}
	return Result{}
}
