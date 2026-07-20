package planner

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/tools/gitops"
)

func buildGitOps(in intent.Intent, ns string) (ExecutionPlan, error) {
	action, _ := in.StringParam("action")
	action = strings.ToLower(strings.TrimSpace(action))
	if action == "" {
		action = "status"
	}
	engine, _ := in.StringParam("engine")
	engine = strings.ToLower(strings.TrimSpace(engine))
	if engine == "" {
		engine = "auto"
	}
	name := strings.TrimSpace(in.Target.Name)
	kind := strings.TrimSpace(in.Target.Kind)

	switch action {
	case "status", "health", "show", "list":
		if kind == "" {
			kind = "Application"
		}
		summary := "GitOps sync/health status"
		switch {
		case name != "" && ns != "":
			summary = fmt.Sprintf("GitOps status for %s/%s -n %s", kind, name, ns)
		case ns != "":
			summary = fmt.Sprintf("GitOps status in namespace %s", ns)
		default:
			summary = "GitOps sync/health status (cluster-wide)"
		}
		if engine != "" && engine != "auto" {
			summary += " (" + engine + ")"
		}
		apiVersion := gitops.ArgoCDGroup + "/v1alpha1"
		if engine == "flux" || strings.EqualFold(kind, "Kustomization") {
			apiVersion = gitops.FluxGroup + "/v1"
			kind = gitops.FluxKind
		} else if engine == "argocd" {
			kind = gitops.ArgoCDKind
		}
		return ExecutionPlan{
			Intent: in,
			Actions: []Action{{
				Op:      OpGitOpsStatus,
				Backend: "gitops",
				Object: ObjectRef{
					APIVersion: apiVersion,
					Kind:       kind,
					Name:       name,
					Namespace:  ns,
				},
				Diff: "list Flux Kustomizations / Argo CD Applications and report sync, health, revision",
			}},
			Summary:          summary,
			RequiresApproval: false,
		}, nil

	case "sync", "promote", "rollback", "reconcile":
		if name == "" {
			return ExecutionPlan{}, fmt.Errorf("gitops %s requires a named Application or Kustomization", action)
		}
		if engine == "" || engine == "auto" {
			return ExecutionPlan{}, fmt.Errorf("gitops %s requires engine=flux or engine=argocd (got %q)", action, engine)
		}
		if action == "reconcile" {
			action = "sync"
		}
		apiVersion := gitops.ArgoCDGroup + "/v1alpha1"
		kind = gitops.ArgoCDKind
		if engine == "flux" {
			apiVersion = gitops.FluxGroup + "/v1"
			kind = gitops.FluxKind
		}
		in.Params["action"] = action
		in.Params["engine"] = engine
		in.Target.Kind = kind

		summary := fmt.Sprintf("GitOps %s %s/%s -n %s (%s)", action, kind, name, ns, engine)
		diff := fmt.Sprintf("%s %s %s/%s", engine, action, kind, name)
		return ExecutionPlan{
			Intent: in,
			Actions: []Action{{
				Op:      OpGitOpsSync,
				Backend: "gitops",
				Object: ObjectRef{
					APIVersion: apiVersion,
					Kind:       kind,
					Name:       name,
					Namespace:  ns,
				},
				Diff: diff,
			}},
			Summary:          summary,
			RequiresApproval: true,
		}, nil

	default:
		return ExecutionPlan{}, fmt.Errorf("unsupported gitops action %q (status|sync|promote|rollback)", action)
	}
}
