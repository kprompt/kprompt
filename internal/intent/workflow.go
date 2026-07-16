package intent

import "regexp"

var workflowPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bargo\s+workflow`),
	regexp.MustCompile(`(?i)\bworkflow\b`),
	regexp.MustCompile(`(?i)\btrain\s+.+\s+model\b`),
	regexp.MustCompile(`(?i)\bsubmit\s+(a\s+)?workflow\b`),
}

// LooksLikeWorkflowPrompt reports natural-language that will need Argo Workflows (T-028 preflight).
func LooksLikeWorkflowPrompt(prompt string) bool {
	p := prompt
	for _, re := range workflowPromptPatterns {
		if re.MatchString(p) {
			return true
		}
	}
	return false
}
