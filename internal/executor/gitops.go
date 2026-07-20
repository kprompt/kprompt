package executor

import (
	"context"
	"fmt"
	"strings"

	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/gitops"
)

// IsGitOpsSyncPlan reports whether the plan only contains GitOps sync/promote/rollback actions.
func IsGitOpsSyncPlan(plan planner.ExecutionPlan) bool {
	if len(plan.Actions) == 0 {
		return false
	}
	for _, a := range plan.Actions {
		if a.Backend != "gitops" || a.Op != planner.OpGitOpsSync {
			return false
		}
	}
	return true
}

// ApplyGitOpsSync triggers Flux reconcile or Argo CD sync for approved plans.
func ApplyGitOpsSync(ctx context.Context, cfg *rest.Config, plan planner.ExecutionPlan) (gitops.SyncResult, error) {
	if cfg == nil {
		return gitops.SyncResult{}, fmt.Errorf("gitops apply: rest config is nil")
	}
	var last gitops.SyncResult
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpGitOpsSync:
			engine, _ := plan.Intent.StringParam("engine")
			action, _ := plan.Intent.StringParam("action")
			revision, _ := plan.Intent.StringParam("revision")
			if engine == "" {
				engine = strings.ToLower(a.Object.Kind)
				switch {
				case strings.EqualFold(a.Object.Kind, gitops.FluxKind):
					engine = "flux"
				case strings.EqualFold(a.Object.Kind, gitops.ArgoCDKind):
					engine = "argocd"
				}
			}
			if action == "" {
				action = "sync"
			}
			ns := a.Object.Namespace
			if ns == "" {
				ns = "default"
			}
			st, err := gitops.TriggerSync(ctx, cfg, gitops.SyncRequest{
				Engine:    engine,
				Name:      a.Object.Name,
				Namespace: ns,
				Action:    action,
				Revision:  revision,
			})
			if err != nil {
				return last, err
			}
			last = st
		default:
			return last, fmt.Errorf("executor: unsupported gitops op %q", a.Op)
		}
	}
	return last, nil
}
