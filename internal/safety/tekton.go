package safety

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

var tektonWipePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\b(delete|remove|wipe)\b.*\b(all|every)\b.*\b(pipeline|pipelinerun)`),
	regexp.MustCompile(`(?i)\b(pipeline|pipelinerun)s?\b.*\b(delete|remove|wipe)\b.*\b(all|every)\b`),
}

// CheckTektonPrompt denies wipe-class Tekton prompts.
func CheckTektonPrompt(prompt string) Result {
	p := strings.TrimSpace(prompt)
	for _, re := range tektonWipePatterns {
		if re.MatchString(p) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: "🛡️ Refusing wipe-class Tekton delete — name a single PipelineRun",
			}
		}
	}
	return Result{Risk: RiskLow}
}

func evaluateTektonPlan(plan planner.ExecutionPlan) Result {
	for _, a := range plan.Actions {
		if a.Backend != "tekton" {
			continue
		}
		if a.Op == planner.OpPipelineRunCreate {
			continue
		}
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: fmt.Sprintf("🛡️ Refusing unsupported Tekton action %q", a.Op),
		}
	}
	return Result{Risk: RiskLow}
}
