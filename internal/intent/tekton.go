package intent

import (
	"regexp"
	"strings"

	"github.com/kprompt/kprompt/internal/tools/tekton"
)

var tektonPromptPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\btekton\b`),
	regexp.MustCompile(`(?i)\b(create|make|run)\b.*\b(ci\s+)?pipeline\b`),
	regexp.MustCompile(`(?i)\bci\s+pipeline\b`),
	regexp.MustCompile(`(?i)\bpipeline\s*run\b`),
	regexp.MustCompile(`(?i)\bpipelinerun\b`),
}

// LooksLikeTektonPrompt reports natural-language that needs Tekton Pipelines (T-039).
func LooksLikeTektonPrompt(prompt string) bool {
	for _, re := range tektonPromptPatterns {
		if re.MatchString(prompt) {
			return true
		}
	}
	return false
}

// NormalizeTekton maps CI/Tekton-shaped prompts onto kind=tekton.
func NormalizeTekton(in Intent, prompt string) Intent {
	if !LooksLikeTektonPrompt(prompt) {
		return in
	}
	switch in.Kind {
	case KindDeploy, KindWorkflow, KindUnknown, KindTekton, KindGet:
		in.Kind = KindTekton
	default:
		if in.Kind != KindTekton {
			return in
		}
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	if _, ok := in.Task(); !ok {
		in.Params["task"] = "ci"
	}
	if _, ok := in.RepoURL(); !ok {
		if repo := tekton.InferRepoFromPrompt(prompt); repo != "" {
			in.Params["repo_url"] = repo
		}
	}
	if strings.TrimSpace(in.Target.Name) == "" {
		task, _ := in.Task()
		repo, _ := in.RepoURL()
		if repo == "" {
			repo, _ = in.Repo()
		}
		in.Target.Name = tekton.DefaultPipelineRunName(task, repo)
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "PipelineRun"
	}
	return in
}
