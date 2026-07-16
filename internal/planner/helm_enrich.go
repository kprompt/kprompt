package planner

import (
	"context"
	"fmt"
	"strings"

	toolshelm "github.com/kprompt/kprompt/internal/tools/helm"
)

// EnrichHelmPlan attaches helm template / dry-run previews to install and upgrade actions.
func EnrichHelmPlan(ctx context.Context, plan *ExecutionPlan) {
	if plan == nil {
		return
	}
	repoURL := helmRepoURL(plan.Actions)
	for i := range plan.Actions {
		a := &plan.Actions[i]
		switch a.Op {
		case OpHelmInstall:
			previewCmd, err := toolshelm.PreviewInstallCommand(a.Command, repoURL)
			if err != nil {
				continue
			}
			body, err := toolshelm.RunCapture(ctx, previewCmd)
			if err != nil {
				a.Diff = appendPreviewNote(a.Diff, fmt.Sprintf("preview unavailable: %v", err))
				continue
			}
			a.Manifest = toolshelm.TruncatePreview(body)
			a.Diff = appendPreviewNote(a.Diff, "helm template preview attached")
		case OpHelmUpgrade:
			previewCmd, err := toolshelm.PreviewUpgradeCommand(a.Command)
			if err != nil {
				continue
			}
			body, err := toolshelm.RunCapture(ctx, previewCmd)
			if err != nil {
				a.Diff = appendPreviewNote(a.Diff, fmt.Sprintf("dry-run preview unavailable: %v", err))
				continue
			}
			a.Manifest = toolshelm.TruncatePreview(body)
			a.Diff = appendPreviewNote(a.Diff, "helm upgrade --dry-run=client preview attached")
		}
	}
}

func helmRepoURL(actions []Action) string {
	for _, a := range actions {
		if a.Op == OpHelmRepo {
			if url := toolshelm.RepoURLFromCommand(a.Command); url != "" {
				return url
			}
		}
	}
	return ""
}

func appendPreviewNote(diff, note string) string {
	diff = strings.TrimSpace(diff)
	if diff == "" {
		return note
	}
	return diff + "\n" + note
}
