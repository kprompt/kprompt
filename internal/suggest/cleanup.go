package suggest

import (
	"context"
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

// orphanConfirmPhrases unlock ConfigMap/Secret orphan delete plans when present
// (case-insensitive) in the original cleanup prompt.
var orphanConfirmPhrases = []string{
	"confirm orphans",
	"confirm orphan",
	"delete unused configmaps",
	"delete unused secrets",
	"delete orphans",
	"and delete orphans",
}

// ConfirmsOrphans reports whether the prompt unlocks ConfigMap/Secret orphan deletes.
func ConfirmsOrphans(prompt string) bool {
	lower := strings.ToLower(prompt)
	for _, p := range orphanConfirmPhrases {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

// FromCleanup turns cleanup Investigation findings into approve-gated delete
// plans for completed Jobs and superseded ReplicaSets. ConfigMap/Secret orphans
// stay guidance-only unless the prompt confirms orphans (stricter gate).
func FromCleanup(_ context.Context, inv incident.Investigation, prompt string) ([]Suggestion, error) {
	var actions []planner.Action
	var orphanActions []planner.Action
	seenDelete := map[string]bool{}
	seenOrphan := map[string]bool{}
	seenGuidance := map[string]bool{}
	var guidance []Suggestion
	confirmOrphans := ConfirmsOrphans(prompt)

	for _, f := range inv.Findings {
		ref := auditResource(f)
		if ref == nil || ref.Name == "" {
			continue
		}
		switch f.Code {
		case "Cleanup.CompletedJob":
			if ref.Kind != "Job" {
				continue
			}
			key := "Job|" + ref.Namespace + "|" + ref.Name
			if seenDelete[key] {
				continue
			}
			seenDelete[key] = true
			actions = append(actions, planner.Action{
				Op: planner.OpDelete,
				Object: planner.ObjectRef{
					APIVersion: "batch/v1",
					Kind:       "Job",
					Name:       ref.Name,
					Namespace:  ref.Namespace,
				},
				Diff: fmt.Sprintf("- Job/%s -n %s (completed, stale)", ref.Name, ref.Namespace),
			})
		case "Cleanup.OldReplicaSet":
			if ref.Kind != "ReplicaSet" {
				continue
			}
			key := "ReplicaSet|" + ref.Namespace + "|" + ref.Name
			if seenDelete[key] {
				continue
			}
			seenDelete[key] = true
			actions = append(actions, planner.Action{
				Op: planner.OpDelete,
				Object: planner.ObjectRef{
					APIVersion: "apps/v1",
					Kind:       "ReplicaSet",
					Name:       ref.Name,
					Namespace:  ref.Namespace,
				},
				Diff: fmt.Sprintf("- ReplicaSet/%s -n %s (superseded, 0 replicas)", ref.Name, ref.Namespace),
			})
		case "Cleanup.UnusedPVC", "Cleanup.EmptyService":
			addAuditGuidance(&guidance, seenGuidance, f.Code,
				cleanupGuidanceTitle(f.Code),
				fmt.Sprintf("describe %s/%s", strings.ToLower(ref.Kind), ref.Name),
				cleanupGuidanceSummary(f.Code))
		case "Cleanup.UnusedConfigMap", "Cleanup.UnusedSecret":
			if !confirmOrphans {
				addAuditGuidance(&guidance, seenGuidance, f.Code,
					cleanupGuidanceTitle(f.Code),
					fmt.Sprintf("describe %s/%s", strings.ToLower(ref.Kind), ref.Name),
					cleanupGuidanceSummary(f.Code))
				continue
			}
			key := ref.Kind + "|" + ref.Namespace + "|" + ref.Name
			if seenOrphan[key] {
				continue
			}
			seenOrphan[key] = true
			apiVersion := "v1"
			diffReason := "unused ConfigMap orphan"
			if ref.Kind == "Secret" {
				diffReason = "unused Secret orphan"
			}
			orphanActions = append(orphanActions, planner.Action{
				Op: planner.OpDelete,
				Object: planner.ObjectRef{
					APIVersion: apiVersion,
					Kind:       ref.Kind,
					Name:       ref.Name,
					Namespace:  ref.Namespace,
				},
				Diff: fmt.Sprintf("- %s/%s -n %s (%s)", ref.Kind, ref.Name, ref.Namespace, diffReason),
			})
		default:
			addAuditGuidance(&guidance, seenGuidance, f.Code,
				"Review cleanup candidate",
				fmt.Sprintf("describe %s", ref.Name),
				f.Message)
		}
	}

	var out []Suggestion
	if len(actions) > 0 {
		plan := &planner.ExecutionPlan{
			Intent: intent.Intent{
				Kind:   intent.KindDelete,
				Target: intent.Target{Kind: "Job", Namespace: inv.Namespace},
				Params: map[string]any{"reason": "Cleanup"},
			},
			Actions:          actions,
			Summary:          fmt.Sprintf("Delete %d stale Job/ReplicaSet cleanup candidate(s)", len(actions)),
			RequiresApproval: true,
		}
		out = append(out, Suggestion{
			Code:    "Cleanup.Delete",
			Title:   "Delete stale Jobs / ReplicaSets",
			Prompt:  "cleanup delete stale jobs and replicasets",
			Plan:    plan,
			Summary: plan.Summary,
		})
	}
	if len(orphanActions) > 0 {
		plan := &planner.ExecutionPlan{
			Intent: intent.Intent{
				Kind:   intent.KindDelete,
				Target: intent.Target{Kind: "ConfigMap", Namespace: inv.Namespace},
				Params: map[string]any{
					"reason":          "CleanupOrphans",
					"confirm_orphans": true,
				},
			},
			Actions:          orphanActions,
			Summary:          fmt.Sprintf("Delete %d unused ConfigMap/Secret orphan(s) (confirm_orphans)", len(orphanActions)),
			RequiresApproval: true,
		}
		out = append(out, Suggestion{
			Code:    "Cleanup.DeleteOrphans",
			Title:   "Delete unused ConfigMap/Secret orphans",
			Prompt:  "cleanup confirm orphans",
			Plan:    plan,
			Summary: plan.Summary,
		})
	}
	out = append(out, guidance...)
	return out, nil
}

// IsCleanupOrphanPlan reports whether a plan is the confirm_orphans ConfigMap/Secret delete.
func IsCleanupOrphanPlan(plan planner.ExecutionPlan) bool {
	if plan.Intent.Kind != intent.KindDelete {
		return false
	}
	v, ok := plan.Intent.Params["confirm_orphans"]
	if !ok || v == nil {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return strings.EqualFold(b, "true")
	default:
		return false
	}
}

func cleanupGuidanceTitle(code string) string {
	switch code {
	case "Cleanup.UnusedConfigMap":
		return "Review unused ConfigMap"
	case "Cleanup.UnusedSecret":
		return "Review unused Secret"
	case "Cleanup.UnusedPVC":
		return "Review unused PVC"
	case "Cleanup.EmptyService":
		return "Review empty Service"
	default:
		return "Review cleanup candidate"
	}
}

func cleanupGuidanceSummary(code string) string {
	switch code {
	case "Cleanup.UnusedConfigMap":
		return "ConfigMaps stay guidance-only unless you confirm orphans (e.g. \"cleanup … and confirm orphans\"); CRD/GitOps refs may be unscanned"
	case "Cleanup.UnusedSecret":
		return "Secrets stay guidance-only unless you confirm orphans; false positives are common — confirm then delete"
	case "Cleanup.UnusedPVC":
		return "PersistentVolumeClaim is not Bound; verify if it is needed or if storage provisioning failed"
	case "Cleanup.EmptyService":
		return "Service has a selector but zero active endpoints; traffic to this service will fail"
	default:
		return "Review the finding before deleting"
	}
}
