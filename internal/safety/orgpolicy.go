package safety

import (
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/planner"
)

// OrgPolicy is the tighten-only org overlay from the Team control plane.
// Local hard-denies always win; org rules may only add restrictions.
type OrgPolicy struct {
	OrgID           string
	Version         int
	MaxRisk         string // low|medium|high
	DenyIntents     []string
	AllowNamespaces []string
	DenyNamespaces  []string
	RequireApprove  bool
}

func riskRank(r Risk) int {
	switch r {
	case RiskLow:
		return 1
	case RiskMedium:
		return 2
	case RiskHigh:
		return 3
	case RiskDenied:
		return 4
	default:
		return 3
	}
}

func parseMaxRisk(s string) Risk {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "low":
		return RiskLow
	case "high":
		return RiskHigh
	case "medium", "":
		return RiskMedium
	default:
		return RiskMedium
	}
}

// EvaluatePlanWithOrg runs local EvaluatePlan then applies org tighten-only rules.
func EvaluatePlanWithOrg(plan planner.ExecutionPlan, org *OrgPolicy) Result {
	base := EvaluatePlan(plan)
	return ApplyOrgPolicy(base, plan, org)
}

// ApplyOrgPolicy tightens a local safety result with org policy (never loosens).
func ApplyOrgPolicy(base Result, plan planner.ExecutionPlan, org *OrgPolicy) Result {
	if org == nil {
		return base
	}
	if base.Denied {
		return base
	}

	ns := planNamespace(plan)
	if ns != "" {
		for _, d := range org.DenyNamespaces {
			if nsMatch(d, ns) {
				return Result{
					Risk:    RiskDenied,
					Denied:  true,
					Message: fmt.Sprintf("🛡️ Org policy denies namespace %q", ns),
				}
			}
		}
		if !namespaceAllowed(org.AllowNamespaces, ns) {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: fmt.Sprintf("🛡️ Org policy does not allow namespace %q", ns),
			}
		}
	}

	kind := strings.ToLower(string(plan.Intent.Kind))
	for _, d := range org.DenyIntents {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || d == "wipe" || d == "delete_cluster" {
			// Always enforced locally as hard-denies; not used to blanket-deny KindDelete.
			continue
		}
		if d == kind {
			return Result{
				Risk:    RiskDenied,
				Denied:  true,
				Message: fmt.Sprintf("🛡️ Org policy denies intent %q", plan.Intent.Kind),
			}
		}
	}

	max := parseMaxRisk(org.MaxRisk)
	if riskRank(base.Risk) > riskRank(max) {
		return Result{
			Risk:    RiskDenied,
			Denied:  true,
			Message: fmt.Sprintf("🛡️ Org policy max_risk is %s — plan risk %s exceeds it", max, base.Risk),
		}
	}

	if org.RequireApprove && riskRank(base.Risk) >= riskRank(RiskMedium) {
		if base.Message == "" {
			base.Message = "Org policy requires approval"
		}
	}
	return base
}

func planNamespace(plan planner.ExecutionPlan) string {
	if ns := strings.TrimSpace(plan.Intent.Target.Namespace); ns != "" {
		return ns
	}
	for _, a := range plan.Actions {
		if ns := strings.TrimSpace(a.Object.Namespace); ns != "" {
			return ns
		}
	}
	return ""
}

func namespaceAllowed(allow []string, ns string) bool {
	if len(allow) == 0 {
		return true
	}
	for _, a := range allow {
		if nsMatch(a, ns) {
			return true
		}
	}
	return false
}

func nsMatch(pattern, ns string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	return strings.EqualFold(pattern, ns)
}
