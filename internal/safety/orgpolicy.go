package safety

import (
	"fmt"
	"strconv"
	"strings"
	"time"

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
	ChangeWindows   []ChangeWindow
	ApproveByRole   map[string][]string
}

// ChangeWindow restricts mutating plans for matching kube contexts (A-070).
type ChangeWindow struct {
	Contexts []string // glob; empty = all contexts; trailing * suffix supported
	TZ       string   // IANA
	Days     []string // mon…sun (normalized; mon-fri expanded on pull)
	Start    string   // HH:MM
	End      string   // HH:MM same-day
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
func EvaluatePlanWithOrg(plan planner.ExecutionPlan, org *OrgPolicy, kubeContext string) Result {
	base := EvaluatePlan(plan)
	return ApplyOrgPolicy(base, plan, org, kubeContext)
}

// ApplyOrgPolicy tightens a local safety result with org policy (never loosens).
func ApplyOrgPolicy(base Result, plan planner.ExecutionPlan, org *OrgPolicy, kubeContext string) Result {
	return ApplyOrgPolicyAt(base, plan, org, kubeContext, time.Now())
}

// ApplyOrgPolicyAt is ApplyOrgPolicy with an injectable clock (tests).
func ApplyOrgPolicyAt(base Result, plan planner.ExecutionPlan, org *OrgPolicy, kubeContext string, now time.Time) Result {
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

	if denied, msg := denyOutsideChangeWindow(plan, base, org.ChangeWindows, kubeContext, now); denied {
		return Result{Risk: RiskDenied, Denied: true, Message: msg}
	}

	if org.RequireApprove && riskRank(base.Risk) >= riskRank(RiskMedium) {
		if base.Message == "" {
			base.Message = "Org policy requires approval"
		}
	}
	return base
}

func denyOutsideChangeWindow(plan planner.ExecutionPlan, base Result, windows []ChangeWindow, kubeContext string, now time.Time) (bool, string) {
	if len(windows) == 0 {
		return false, ""
	}
	// Reads stay allowed outside windows; mutate/medium+ only.
	if !plan.RequiresApproval && riskRank(base.Risk) < riskRank(RiskMedium) {
		return false, ""
	}

	matching := make([]ChangeWindow, 0)
	for _, w := range windows {
		if contextMatchesWindow(w.Contexts, kubeContext) {
			matching = append(matching, w)
		}
	}
	if len(matching) == 0 {
		return false, "" // no window claims this context
	}
	for _, w := range matching {
		if windowOpen(w, now) {
			return false, ""
		}
	}
	return true, fmt.Sprintf(
		"🛡️ Org change window: mutate on context %q is outside allowed hours",
		strings.TrimSpace(kubeContext),
	)
}

func contextMatchesWindow(patterns []string, kubeContext string) bool {
	ctx := strings.TrimSpace(kubeContext)
	if len(patterns) == 0 {
		return true
	}
	for _, p := range patterns {
		if contextGlobMatch(p, ctx) {
			return true
		}
	}
	return false
}

func contextGlobMatch(pattern, name string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	name = strings.TrimSpace(name)
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(strings.ToLower(name), strings.ToLower(prefix))
	}
	return strings.EqualFold(pattern, name)
}

var weekdayShort = map[time.Weekday]string{
	time.Sunday:    "sun",
	time.Monday:    "mon",
	time.Tuesday:   "tue",
	time.Wednesday: "wed",
	time.Thursday:  "thu",
	time.Friday:    "fri",
	time.Saturday:  "sat",
}

func windowOpen(w ChangeWindow, now time.Time) bool {
	loc, err := time.LoadLocation(strings.TrimSpace(w.TZ))
	if err != nil {
		return false
	}
	local := now.In(loc)
	day := weekdayShort[local.Weekday()]
	dayOK := false
	for _, d := range w.Days {
		if strings.EqualFold(strings.TrimSpace(d), day) {
			dayOK = true
			break
		}
	}
	if !dayOK {
		return false
	}
	sm, err1 := parseClockMinutes(w.Start)
	em, err2 := parseClockMinutes(w.End)
	if err1 != nil || err2 != nil || sm >= em {
		return false
	}
	mins := local.Hour()*60 + local.Minute()
	return mins >= sm && mins < em
}

func parseClockMinutes(s string) (int, error) {
	parts := strings.Split(strings.TrimSpace(s), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("bad time")
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, err
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, err
	}
	return h*60 + m, nil
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

// RoleMayApprove reports whether role may approve a plan at the given risk (A-071).
// Empty matrix = no role×risk constraint (allow).
func RoleMayApprove(matrix map[string][]string, role string, risk Risk) bool {
	if len(matrix) == 0 {
		return true
	}
	if risk == RiskDenied || risk == "" {
		return false
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		return false
	}
	allowed, ok := matrix[role]
	if !ok {
		return false
	}
	want := strings.ToLower(strings.TrimSpace(string(risk)))
	for _, a := range allowed {
		if strings.EqualFold(strings.TrimSpace(a), want) {
			return true
		}
	}
	return false
}

// RoleApproveDenyMessage returns a deny message when the member role cannot approve risk.
func RoleApproveDenyMessage(matrix map[string][]string, role string, risk Risk) string {
	if RoleMayApprove(matrix, role, risk) {
		return ""
	}
	role = strings.ToLower(strings.TrimSpace(role))
	if role == "" {
		role = "(unknown)"
	}
	return fmt.Sprintf("🛡️ Org policy: role %q may not approve risk %s", role, risk)
}
