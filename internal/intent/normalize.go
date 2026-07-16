package intent

import (
	"strings"

	"github.com/kprompt/kprompt/internal/tools/argo"
)

// NormalizeVerb adjusts intent kind using prompt phrasing when the model confuses similar verbs.
func NormalizeVerb(in Intent, prompt string) Intent {
	p := strings.ToLower(strings.TrimSpace(prompt))
	if strings.HasPrefix(p, "install ") && in.Kind == KindDeploy {
		in.Kind = KindInstall
	}
	return NormalizeWorkflow(in, prompt)
}

// NormalizeWorkflow maps workflow-shaped prompts to workflow intent and fills params.
func NormalizeWorkflow(in Intent, prompt string) Intent {
	if !LooksLikeWorkflowPrompt(prompt) {
		return in
	}
	p := strings.ToLower(strings.TrimSpace(prompt))
	if in.Kind == KindDeploy || in.Kind == KindUnknown {
		in.Kind = KindWorkflow
	}
	if in.Kind != KindWorkflow {
		return in
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	if _, ok := in.Task(); !ok {
		if strings.Contains(p, "train") {
			in.Params["task"] = "train"
		}
	}
	if _, ok := in.Model(); !ok {
		if m := argo.InferModelFromPrompt(prompt); m != "" {
			in.Params["model"] = m
		}
	}
	if strings.TrimSpace(in.Target.Name) == "" {
		task, _ := in.Task()
		if model, ok := in.Model(); ok {
			in.Target.Name = argo.DefaultWorkflowName(task, model)
		}
	}
	if strings.TrimSpace(in.Target.Kind) == "" {
		in.Target.Kind = "Workflow"
	}
	return in
}
