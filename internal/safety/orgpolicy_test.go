package safety

import (
	"strings"
	"testing"
	"time"

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
	r := ApplyOrgPolicy(base, plan, org, "")
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
	r := ApplyOrgPolicy(base, plan, org, "")
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
	r := ApplyOrgPolicy(base, plan, org, "")
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
	r := ApplyOrgPolicy(base, plan, org, "")
	if !r.Denied {
		t.Fatalf("expected scale deny: %+v", r)
	}
}

func TestApplyOrgPolicyCannotLoosenLocalHardDeny(t *testing.T) {
	tests := []struct {
		name string
		plan planner.ExecutionPlan
	}{
		{
			name: "wipe intent stays denied",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{
					Kind: intent.KindDeny,
				},
			},
		},
		{
			name: "namespace delete stays denied",
			plan: planner.ExecutionPlan{
				Intent: intent.Intent{
					Kind:   intent.KindDelete,
					Target: intent.Target{Kind: "Namespace", Name: "prod"},
				},
				Actions: []planner.Action{{
					Op:     planner.OpDelete,
					Object: planner.ObjectRef{Kind: "Namespace", Name: "prod"},
				}},
			},
		},
	}

	org := &OrgPolicy{
		MaxRisk:         "high",
		AllowNamespaces: []string{"*"},
		RequireApprove:  false,
		DenyIntents:     nil,
		ChangeWindows: []ChangeWindow{{
			Contexts: []string{"*"},
			TZ:       "UTC",
			Days:     []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			Start:    "00:00",
			End:      "23:59",
		}},
		ApproveByRole: map[string][]string{
			"viewer":   {"low", "medium", "high"},
			"operator": {"low", "medium", "high"},
			"admin":    {"low", "medium", "high"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			base := EvaluatePlan(tc.plan)
			if !base.Denied {
				t.Fatalf("expected local hard-deny base result, got %+v", base)
			}
			got := ApplyOrgPolicy(base, tc.plan, org, "prod-main")
			if got != base {
				t.Fatalf("org policy must not loosen or rewrite local deny: got %+v want %+v", got, base)
			}
		})
	}
}

func TestApplyOrgPolicyNilPassthrough(t *testing.T) {
	plan := planner.ExecutionPlan{Intent: intent.Intent{Kind: intent.KindGet}}
	base := EvaluatePlan(plan)
	r := ApplyOrgPolicy(base, plan, nil, "")
	if r != base {
		t.Fatalf("nil org should passthrough")
	}
}

func TestChangeWindowDeniesOutsideHours(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent:            intent.Intent{Kind: intent.KindScale, Target: intent.Target{Namespace: "default", Name: "api"}},
		RequiresApproval:  true,
	}
	base := EvaluatePlan(plan)
	org := &OrgPolicy{
		MaxRisk:         "high",
		AllowNamespaces: []string{"*"},
		ChangeWindows: []ChangeWindow{{
			Contexts: []string{"prod*"},
			TZ:       "UTC",
			Days:     []string{"mon", "tue", "wed", "thu", "fri", "sat", "sun"},
			Start:    "09:00",
			End:      "17:00",
		}},
	}
	// Monday 08:00 UTC — before window
	now := time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	r := ApplyOrgPolicyAt(base, plan, org, "prod-west", now)
	if !r.Denied {
		t.Fatalf("expected outside-window deny: %+v", r)
	}
	// Monday 10:00 UTC — inside
	now = time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)
	r = ApplyOrgPolicyAt(base, plan, org, "prod-west", now)
	if r.Denied {
		t.Fatalf("expected allow inside window: %+v", r)
	}
	// Unrelated context — no window claims it
	now = time.Date(2026, 7, 27, 8, 0, 0, 0, time.UTC)
	r = ApplyOrgPolicyAt(base, plan, org, "staging", now)
	if r.Denied {
		t.Fatalf("unmatched context should pass: %+v", r)
	}
	// Read stays allowed outside window
	read := planner.ExecutionPlan{Intent: intent.Intent{Kind: intent.KindGet}}
	rb := EvaluatePlan(read)
	r = ApplyOrgPolicyAt(rb, read, org, "prod-west", now)
	if r.Denied {
		t.Fatalf("reads should pass outside window: %+v", r)
	}
}

func TestRoleMayApprove(t *testing.T) {
	matrix := map[string][]string{
		"viewer":   {},
		"operator": {"low", "medium"},
		"admin":    {"low", "medium", "high"},
	}
	if RoleMayApprove(matrix, "viewer", RiskMedium) {
		t.Fatal("viewer must not approve medium")
	}
	if !RoleMayApprove(matrix, "operator", RiskMedium) {
		t.Fatal("operator may approve medium")
	}
	if RoleMayApprove(matrix, "operator", RiskHigh) {
		t.Fatal("operator must not approve high")
	}
	if !RoleMayApprove(matrix, "admin", RiskHigh) {
		t.Fatal("admin may approve high")
	}
	if !RoleMayApprove(nil, "viewer", RiskHigh) {
		t.Fatal("empty matrix = no constraint")
	}
	if RoleMayApprove(matrix, "auditor", RiskLow) {
		t.Fatal("unknown role must fail closed")
	}
	if RoleMayApprove(matrix, "admin", RiskDenied) {
		t.Fatal("denied risk must never be approvable")
	}
	if RoleMayApprove(matrix, "", RiskLow) {
		t.Fatal("empty role must fail closed")
	}
	if msg := RoleApproveDenyMessage(matrix, "viewer", RiskLow); msg == "" {
		t.Fatal("expected deny message for viewer/low")
	}
	if msg := RoleApproveDenyMessage(matrix, "auditor", RiskHigh); msg == "" {
		t.Fatal("expected deny message for unknown role/high")
	}
}

func TestApplyOrgPolicyWontWaiveLocalWipeDeny(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindDelete},
		Actions: []planner.Action{{
			Op:     planner.OpDelete,
			Object: planner.ObjectRef{Kind: "Pod", Name: "*", Namespace: "default"},
		}},
	}
	base := EvaluatePlan(plan)
	if !base.Denied {
		t.Fatal("expected local plan to be denied natively (unscoped delete)")
	}

	org := &OrgPolicy{
		MaxRisk:         "high",
		AllowNamespaces: []string{"*"},
	}
	r := ApplyOrgPolicy(base, plan, org, "")
	if !r.Denied {
		t.Fatal("org policy must not be able to waive local unscoped delete deny")
	}
}

func TestApplyOrgPolicyWontWaiveLocalNamespaceDeny(t *testing.T) {
	plan := planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindDelete},
		Actions: []planner.Action{{
			Op:     planner.OpDelete,
			Object: planner.ObjectRef{Kind: "Namespace", Name: "prod"},
		}},
	}
	base := EvaluatePlan(plan)
	if !base.Denied {
		t.Fatal("expected local plan to be denied natively (namespace delete)")
	}

	org := &OrgPolicy{
		MaxRisk:         "high",
		AllowNamespaces: []string{"*"},
	}
	r := ApplyOrgPolicy(base, plan, org, "")
	if !r.Denied {
		t.Fatal("org policy must not be able to waive local namespace deletion deny")
	}
}

func TestRoleMayApproveAdversarial(t *testing.T) {
	matrix := map[string][]string{
		"operator": {"low", "medium"},
		"admin":    {"low", "medium", "high"},
	}

	// 1. Role must not be able to approve RiskDenied
	if RoleMayApprove(matrix, "admin", RiskDenied) {
		t.Fatal("admin must not be able to approve RiskDenied")
	}
	if RoleMayApprove(matrix, "admin", "") {
		t.Fatal("admin must not be able to approve empty Risk")
	}

	// 2. Empty or unknown roles must be denied
	if RoleMayApprove(matrix, "", RiskLow) {
		t.Fatal("empty role must not be able to approve")
	}
	if RoleMayApprove(matrix, "unknown-role", RiskLow) {
		t.Fatal("unknown role must not be able to approve")
	}

	// 3. Case insensitivity check
	if !RoleMayApprove(matrix, "Operator", "MEDIUM") {
		t.Fatal("RoleMayApprove should be case-insensitive for role and risk")
	}
	if !RoleMayApprove(matrix, "  operator  ", "  medium  ") {
		t.Fatal("RoleMayApprove should handle surrounding spaces")
	}

	// 4. RoleApproveDenyMessage format check for unknown and unauthorized cases
	if msg := RoleApproveDenyMessage(matrix, "", RiskHigh); !strings.Contains(msg, "(unknown)") {
		t.Fatalf("expected message to reference unknown role, got: %q", msg)
	}
	if msg := RoleApproveDenyMessage(matrix, "operator", RiskHigh); !strings.Contains(msg, "operator") || !strings.Contains(msg, "high") {
		t.Fatalf("expected message to reference role and risk, got: %q", msg)
	}
}

func TestParseMaxRiskDefaults(t *testing.T) {
	if parseMaxRisk("") != RiskMedium {
		t.Fatal("empty max risk should default to RiskMedium")
	}
	if parseMaxRisk("invalid-value") != RiskMedium {
		t.Fatal("invalid max risk should default to RiskMedium")
	}
	if parseMaxRisk("  LoW  ") != RiskLow {
		t.Fatal("max risk parsing should be case-insensitive and trim spaces")
	}
}
