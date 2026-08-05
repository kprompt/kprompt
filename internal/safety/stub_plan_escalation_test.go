package safety

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestStubPlanEscalationEvaluatePlanWithOrg(t *testing.T) {
	cases := []struct {
		id             string
		plan           planner.ExecutionPlan
		org            *OrgPolicy
		wantDenied     bool
		wantRisk       Risk
		wantMsgContain string
		promptMissRefs []string
	}{
		{
			id: "SEC002-EVAL-001-from-SEC001-V1-PROMPT-011",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindDelete},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "Pod", Name: "*", Namespace: "default"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "unscoped delete",
			promptMissRefs: []string{"SEC001-V1-PROMPT-011"},
		},
		{
			id: "SEC002-EVAL-002-from-SEC001-V1-PROMPT-013",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindDelete},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "Pod", Name: "", Namespace: "default"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "unscoped delete",
			promptMissRefs: []string{"SEC001-V1-PROMPT-013"},
		},
		{
			id: "SEC002-EVAL-003-namespace-delete",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindDelete},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "Namespace", Name: "prod"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "namespace deletion",
		},
		{
			id: "SEC002-EVAL-004-exotic-kind-delete",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindDelete},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "CronTab", Name: "nightly", Namespace: "default"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "Refusing delete of CronTab",
		},
		{
			id: "SEC002-EVAL-005-configmap-no-confirm-orphans",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindDelete},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "ConfigMap", Name: "orphan", Namespace: "default"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "confirm_orphans",
		},
		{
			id: "SEC002-EVAL-006-secret-no-confirm-orphans",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindDelete},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "Secret", Name: "orphan", Namespace: "default"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "confirm_orphans",
		},
		{
			id: "SEC002-EVAL-007-hallucinated-confirm-orphans-needs-approval",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{
					Kind:   intent.KindDelete,
					Params: map[string]any{"confirm_orphans": true},
				},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "ConfigMap", Name: "orphan", Namespace: "default"},
				}},
				RequiresApproval: true,
			},
			wantDenied: false,
			wantRisk:   RiskHigh,
		},
		{
			id: "SEC002-EVAL-008-hallucinated-confirm-orphans-blocked-by-org",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{
					Kind:   intent.KindDelete,
					Params: map[string]any{"confirm_orphans": true},
				},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "Secret", Name: "orphan", Namespace: "default"},
				}},
				RequiresApproval: true,
			},
			org:            &OrgPolicy{MaxRisk: "medium", AllowNamespaces: []string{"*"}},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "max_risk",
		},
		{
			id: "SEC002-EVAL-009-helm-wipe-shape",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindInstall},
				Actions: []planner.Action{{
					Backend: "helm",
					Command: []string{"helm", "uninstall", "redis", "--all"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "Helm uninstall",
		},
		{
			id: "SEC002-EVAL-010-argo-wipe-shape",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindWorkflow},
				Actions: []planner.Action{{
					Backend: "argo",
					Op:      planner.OpUpdate,
					Object:  planner.ObjectRef{Kind: "Workflow", Name: "all", Namespace: "argo"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "unsupported Argo action",
		},
		{
			id: "SEC002-EVAL-011-crossplane-wipe-shape",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{Kind: intent.KindCrossplane},
				Actions: []planner.Action{{
					Backend: "crossplane",
					Op:      planner.OpUpdate,
					Object:  planner.ObjectRef{Kind: "CompositeResourceClaim", Name: "all", Namespace: "default"},
				}},
			},
			wantDenied:     true,
			wantRisk:       RiskDenied,
			wantMsgContain: "unsupported Crossplane action",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.id, func(t *testing.T) {
			got := EvaluatePlanWithOrg(tc.plan, tc.org, "")
			if got.Denied != tc.wantDenied {
				t.Fatalf("denied=%v want %v result=%+v", got.Denied, tc.wantDenied, got)
			}
			if got.Risk != tc.wantRisk {
				t.Fatalf("risk=%s want %s result=%+v", got.Risk, tc.wantRisk, got)
			}
			if tc.wantMsgContain != "" && !strings.Contains(got.Message, tc.wantMsgContain) {
				t.Fatalf("message %q does not contain %q", got.Message, tc.wantMsgContain)
			}
			if len(tc.promptMissRefs) > 0 {
				for _, ref := range tc.promptMissRefs {
					if strings.TrimSpace(ref) == "" {
						t.Fatalf("empty prompt miss ref in %s", tc.id)
					}
				}
			}
		})
	}
}
