package suggest

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/drift"
	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/tools/gitops"
)

// FromDrift turns Drift.OutOfSync findings into approve-gated GitOps sync plans
// plus guidance for unhealthy apps, resource-level diffs, and PR mode.
func FromDrift(inv incident.Investigation) ([]Suggestion, error) {
	var out []Suggestion
	seenSync := map[string]bool{}
	seenGuide := map[string]bool{}

	for _, f := range inv.Findings {
		switch f.Code {
		case drift.CodeOutOfSync:
			ref := driftAppRef(f)
			if ref == nil {
				continue
			}
			engine := driftEngine(f, ref.Kind)
			key := engine + "|" + ref.Namespace + "|" + ref.Name
			if seenSync[key] {
				continue
			}
			seenSync[key] = true
			plan := gitopsSyncPlan(engine, ref)
			out = append(out, Suggestion{
				Code:    f.Code,
				Title:   "Sync to clear drift",
				Prompt:  fmt.Sprintf("sync %s %s -n %s", engine, ref.Name, ref.Namespace),
				Plan:    &plan,
				Summary: fmt.Sprintf("Approve-gated %s sync for %s/%s (never silent)", engine, ref.Kind, ref.Name),
			})
		case drift.CodeUnhealthy:
			ref := driftAppRef(f)
			addDriftGuidance(&out, seenGuide, f.Code,
				"Investigate unhealthy GitOps app",
				guidancePrompt(ref, "describe"),
				"Synced-but-unhealthy apps need workload RCA (investigate/why) — sync alone may not help")
		case drift.CodeResourceDrift:
			ref := driftAppRef(f)
			addDriftGuidance(&out, seenGuide, f.Code,
				"Resource differs from Git",
				guidancePrompt(ref, "investigate"),
				"Per-resource OutOfSync entries are reported from Argo inventory — reconcile via app sync or fix Git with kprompt --gitops --gitops-repo owner/name \"...\"")
		case drift.CodeMissing:
			addDriftGuidance(&out, seenGuide, f.Code,
				"Install GitOps to enable drift",
				"learn cluster tools",
				"Install Flux or Argo CD, then re-run drift. For PR mode, see docs/gitops-pr.md and use kprompt --gitops --gitops-repo owner/name \"...\"")
		}
	}

	if len(ActionablePlans(out)) > 0 {
		addDriftGuidance(&out, seenGuide, "Drift.PRMode",
			"Prefer a Git PR instead of live sync?",
			"gitops pr mode",
			"Live sync reconciles toward Git desired state. Prefer kprompt --gitops --gitops-repo owner/name \"...\" to open a PR instead of cluster mutate")
	}
	return out, nil
}

func driftAppRef(f incident.Finding) *incident.ResourceRef {
	for _, e := range f.Evidence {
		if e.Resource != nil && strings.TrimSpace(e.Resource.Name) != "" {
			return e.Resource
		}
	}
	return nil
}

func driftEngine(f incident.Finding, kind string) string {
	for _, e := range f.Evidence {
		src := strings.ToLower(strings.TrimSpace(e.Source))
		if src == "flux" || src == "argocd" || src == "argo" {
			if src == "argo" {
				return "argocd"
			}
			return src
		}
	}
	if strings.EqualFold(kind, gitops.FluxKind) {
		return "flux"
	}
	return "argocd"
}

func gitopsSyncPlan(engine string, ref *incident.ResourceRef) planner.ExecutionPlan {
	ns := ref.Namespace
	if ns == "" {
		ns = "default"
	}
	apiVersion := gitops.ArgoCDGroup + "/v1alpha1"
	kind := gitops.ArgoCDKind
	if engine == "flux" {
		apiVersion = gitops.FluxGroup + "/v1"
		kind = gitops.FluxKind
	}
	in := intent.Intent{
		Kind: intent.KindGitOps,
		Target: intent.Target{
			Kind:      kind,
			Name:      ref.Name,
			Namespace: ns,
		},
		Params: map[string]any{
			"action": "sync",
			"engine": engine,
		},
	}
	return planner.ExecutionPlan{
		Intent: in,
		Actions: []planner.Action{{
			Op:      planner.OpGitOpsSync,
			Backend: "gitops",
			Object: planner.ObjectRef{
				APIVersion: apiVersion,
				Kind:       kind,
				Name:       ref.Name,
				Namespace:  ns,
			},
			Diff: fmt.Sprintf("%s sync %s/%s (from drift suggest)", engine, kind, ref.Name),
		}},
		Summary:          fmt.Sprintf("GitOps sync %s/%s -n %s (%s) to clear drift", kind, ref.Name, ns, engine),
		RequiresApproval: true,
	}
}

func guidancePrompt(ref *incident.ResourceRef, verb string) string {
	if ref == nil {
		return "check drift"
	}
	if ref.Namespace != "" {
		return fmt.Sprintf("%s %s -n %s", verb, ref.Name, ref.Namespace)
	}
	return fmt.Sprintf("%s %s", verb, ref.Name)
}

func addDriftGuidance(out *[]Suggestion, seen map[string]bool, code, title, prompt, summary string) {
	if seen[code] {
		return
	}
	seen[code] = true
	*out = append(*out, Suggestion{
		Code:    code,
		Title:   title,
		Prompt:  prompt,
		Summary: summary,
	})
}
