package safety

import (
	"testing"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestApplyOrgPolicyNamespaceDeny(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{
			Kind: intent.KindScale,
			Target: intent.Target{Namespace: "kube-system", Name: "coredns"},
		},
	}
	base := EvaluatePlan(plan)
	org := &OrgPolicy{
		MaxRisk:         "high",
		DenyNamespaces:  []string{"kube-system"},
		AllowNamespaces: []string{"*"},
	}
	r := ApplyOrgPolicy(base, plan, org)
	if !r.Denied {
		t.Fatalf("expected deny: %+v", r)
	}
}

func TestApplyOrgPolicyAllowList(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{
			Kind: intent.KindScale,
			Target: intent.Target{Namespace: "prod", Name: "api"},
		},
	}
	base := EvaluatePlan(plan)
	org := &OrgPolicy{
		MaxRisk:         "high",
		AllowNamespaces: []string{"staging"},
	}
	r := ApplyOrgPolicy(base, plan, org)
	if !r.Denied {
		t.Fatalf("expected deny outside allow list: %+v", r)
	}
}

func TestApplyOrgPolicyMaxRisk(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindDelete},
		Actions: []planner.Action{{
			Op: planner.OpDelete,
			Object: planner.ObjectRef{Kind: "Deployment", Name: "redis", Namespace: "default"},
		}},
	}
	base := EvaluatePlan(plan) // RiskHigh
	org := &OrgPolicy{MaxRisk: "medium", AllowNamespaces: []string{"*"}}
	r := ApplyOrgPolicy(base, plan, org)
	if !r.Denied {
		t.Fatalf("expected max_risk deny: %+v base=%+v", r, base)
	}
}

func TestApplyOrgPolicyDenyIntent(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{
			Kind:   intent.KindScale,
			Target: intent.Target{Namespace: "default", Name: "api"},
		},
	}
	base := EvaluatePlan(plan)
	org := &OrgPolicy{
		MaxRisk:         "high",
		DenyIntents:     []string{"wipe", "delete_cluster", "scale"},
		AllowNamespaces: []string{"*"},
	}
	r := ApplyOrgPolicy(base, plan, org)
	if !r.Denied {
		t.Fatalf("expected scale deny: %+v", r)
	}
}

func TestApplyOrgPolicyNilPassthrough(t *testing.T) {
	plan := planner.ExecutionPlan{Intent: intent.Intent{Kind: intent.KindGet}}
	base := EvaluatePlan(plan)
	r := ApplyOrgPolicy(base, plan, nil)
	if r != base {
		t.Fatalf("nil org should passthrough")
	}
}
