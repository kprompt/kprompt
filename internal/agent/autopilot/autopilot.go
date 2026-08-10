// Package autopilot implements policy-gated Autopilot proposals and apply (AG-017 · AG-040…AG-044 · ADR-0015).
//
// Default is proposeOnly: PlanResult-shaped proposals + audit, never silent mutate.
// policyAuto apply requires Policy.Apply + ModePolicyAuto + allowlist + confidence + Safety.
package autopilot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"context"

	"github.com/kprompt/kprompt/internal/agent/coordinator"
	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/patterns"
	"github.com/kprompt/kprompt/internal/graph"
	"github.com/kprompt/kprompt/internal/incident"
)

const (
	APIVersion    = "kprompt.io/v1"
	KindProposal  = "AutopilotProposal"
	KindPolicy    = "RemediationPolicy"
	SchemaVersion = "1"

	// Allowlisted action IDs (AG-041). LLM cannot expand this list.
	ActionRollbackFailedRollout = "rollbackFailedRollout"
	ActionRestartDeployment     = "restartDeployment"
	ActionScaleDeployment       = "scaleDeployment"
	ActionEvictPod              = "evictPod"

	DecisionProposed = "proposed"
	DecisionApproved = "approved"
	DecisionDenied   = "denied"
	DecisionApplied  = "applied"
	DecisionFailed   = "failed"

	ModeProposeOnly = "proposeOnly"
	ModePolicyAuto  = "policyAuto"

	DefaultMinConfidence = 0.85

	// ConfigMapName is the optional in-cluster RemediationPolicy store (AG-040).
	ConfigMapName = "kprompt-remediation-policy"
	ConfigMapKey  = "policy.json"
)

// MVPAllowlist is the only action IDs Autopilot may propose/apply (ADR-0015 · AG-041).
var MVPAllowlist = []string{
	ActionRollbackFailedRollout,
	ActionRestartDeployment,
	ActionScaleDeployment,
	ActionEvictPod,
}

// Policy is the per-namespace Autopilot gate (ADR-0015 §4 · AG-040).
type Policy struct {
	APIVersion    string `json:"apiVersion,omitempty"`
	Kind          string `json:"kind,omitempty"`
	SchemaVersion string `json:"schemaVersion,omitempty"`
	Namespace     string `json:"namespace,omitempty"`
	// Allow lists action IDs permitted in this namespace (subset of MVPAllowlist).
	Allow []string `json:"allow"`
	// Apply enables mutate when Mode is policyAuto. Default false.
	Apply bool `json:"apply"`
	// Mode is proposeOnly (default) or policyAuto.
	Mode string `json:"mode,omitempty"`
	// MinConfidence required before propose/apply (default 0.85).
	MinConfidence float64 `json:"minConfidence"`
}

// Normalize fills defaults and clamps Mode.
func (p *Policy) Normalize() {
	if p == nil {
		return
	}
	if p.APIVersion == "" {
		p.APIVersion = APIVersion
	}
	if p.Kind == "" {
		p.Kind = KindPolicy
	}
	if p.SchemaVersion == "" {
		p.SchemaVersion = SchemaVersion
	}
	if p.MinConfidence <= 0 {
		p.MinConfidence = DefaultMinConfidence
	}
	mode := strings.ToLower(strings.TrimSpace(p.Mode))
	switch mode {
	case "policyauto", "policy-auto", "auto":
		p.Mode = ModePolicyAuto
	default:
		p.Mode = ModeProposeOnly
	}
	// Propose-only cannot silently apply even if Apply was set by mistake.
	if p.Mode != ModePolicyAuto {
		p.Apply = false
	}
}

// PolicyAuto reports whether gated in-cluster apply is enabled.
func (p Policy) PolicyAuto() bool {
	p.Normalize()
	return p.Mode == ModePolicyAuto && p.Apply
}

// Proposal is a PlanResult-shaped Autopilot artifact (auditable).
type Proposal struct {
	APIVersion    string   `json:"apiVersion"`
	Kind          string   `json:"kind"`
	SchemaVersion string   `json:"schemaVersion"`
	ID            string   `json:"id"`
	Namespace     string   `json:"namespace"`
	ActionID      string   `json:"actionId"`
	Decision      string   `json:"decision"` // proposed|approved|denied|applied|failed
	Reason        string   `json:"reason,omitempty"`
	Confidence    float64  `json:"confidence"`
	IncidentID    string   `json:"incidentId,omitempty"`
	TargetKind    string   `json:"targetKind,omitempty"`
	TargetName    string   `json:"targetName,omitempty"`
	Plan          PlanBody `json:"plan"`
	Risk          string   `json:"risk"` // low|medium|high|denied
	// AG-041 explainability fields (RecommendedAction-shaped).
	Why              string    `json:"why,omitempty"`
	ExpectedImpact   string    `json:"expectedImpact,omitempty"`
	Rollback         string    `json:"rollback,omitempty"`
	ActionConfidence float64   `json:"actionConfidence,omitempty"`
	Replicas         *int32    `json:"replicas,omitempty"` // scaleDeployment
	Applied          bool      `json:"applied"`
	CreatedAt        time.Time `json:"createdAt"`
	// RT-001 post-apply verify + Learn outcome (success|fail|partial mapped to patterns.Outcome).
	VerifyStatus  string `json:"verifyStatus,omitempty"`
	VerifyMessage string `json:"verifyMessage,omitempty"`
	Outcome       string `json:"outcome,omitempty"` // apply_success | apply_failed | apply_partial
	LearnNote     string `json:"learnNote,omitempty"` // RT-002 ranking / ActionConfidence bias
}

// PlanBody mirrors a minimal PlanResult.plan payload.
type PlanBody struct {
	Summary string   `json:"summary"`
	Steps   []string `json:"steps"`
}

// AuditEntry is appended to the local audit log.
type AuditEntry struct {
	At       time.Time `json:"at"`
	Proposal Proposal  `json:"proposal"`
}

// Engine evaluates policy and emits proposals; ApplyProposal mutates only under policyAuto.
type Engine struct {
	Policy Policy
	Audit  AuditStore
	// Patterns is optional Learn store for RT-002 ranking / ActionConfidence bias (evidence-not-proof).
	Patterns *patterns.Library
	// Graph is optional namespace service-graph snapshot for ExpectedImpact notes (RT-015).
	Graph *graph.Report
	// Fleet is optional Coordinator outcome reader for cross-ns bias (RT-022).
	// Evidence-not-proof: only nudges ActionConfidence when local Learn already
	// matched; never gates apply and never creates candidates (AG-034).
	Fleet coordinator.OutcomeReader
	// FleetTTL caches the fleet summary between fetches. 0 → DefaultFleetTTL.
	FleetTTL time.Duration

	fleetCache   *coordinator.OutcomeSummary
	fleetFetched time.Time

	mu sync.Mutex
}

// AuditStore persists proposals.
type AuditStore interface {
	Append(entry AuditEntry) error
}

// FileAudit writes JSONL under Dir.
type FileAudit struct {
	Dir string
}

func (a FileAudit) path() string {
	return filepath.Join(a.Dir, "autopilot-audit.jsonl")
}

func (a FileAudit) Append(entry AuditEntry) error {
	if err := os.MkdirAll(a.Dir, 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(a.path(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(b, '\n'))
	return err
}

// MemAudit is an in-memory audit for tests.
type MemAudit struct {
	mu      sync.Mutex
	Entries []AuditEntry
}

func (a *MemAudit) Append(entry AuditEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.Entries = append(a.Entries, entry)
	return nil
}

// DefaultPolicy returns propose-only with the expanded allowlist available.
func DefaultPolicy() Policy {
	p := Policy{
		Allow:         append([]string(nil), MVPAllowlist...),
		Apply:         false,
		Mode:          ModeProposeOnly,
		MinConfidence: DefaultMinConfidence,
	}
	p.Normalize()
	return p
}

// EvaluateAction returns denied if hard-deny, outside MVP allowlist, or outside policy Allow.
func EvaluateAction(policy Policy, actionID string) (decision string, reason string) {
	actionID = strings.TrimSpace(actionID)
	if denied, why := HardDenyAction(actionID); denied {
		return DecisionDenied, why
	}
	if !inList(MVPAllowlist, actionID) {
		return DecisionDenied, "hard-deny: action not in Autopilot allowlist (ADR-0015 / AG-041)"
	}
	if !inList(policy.Allow, actionID) {
		return DecisionDenied, "hard-deny: action not in namespace RemediationPolicy allowlist"
	}
	return DecisionApproved, ""
}

// ProposeFromContext builds a proposal when context matches an allowlisted detector.
// Never sets Applied=true here — apply is ApplyProposal only.
// When Engine.Patterns is set, ranks candidates and biases ActionConfidence (RT-002).
func (e *Engine) ProposeFromContext(agentCtx ctxbuild.AgentContext, confidence float64) (*Proposal, error) {
	if e == nil {
		return nil, fmt.Errorf("autopilot: engine is nil")
	}
	pol := e.Policy
	pol.Normalize()
	cands := detectCandidates(agentCtx)
	var match patterns.Pattern
	matched := false
	if e.Patterns != nil {
		match, matched = e.Patterns.Match(agentCtx.Namespace, agentCtx)
		cands = rankCandidates(cands, match, matched)
	}
	if len(cands) == 0 {
		return nil, nil
	}
	top := cands[0]
	action, targetKind, targetName, replicas := top.Action, top.Kind, top.Name, top.Replicas

	decision, reason := EvaluateAction(pol, action)
	if decision == DecisionDenied {
		p := baseProposal(agentCtx, action, targetKind, targetName, confidence, replicas)
		p.Decision = DecisionDenied
		p.Reason = reason
		p.Risk = "denied"
		p.Plan = PlanBody{Summary: "Denied Autopilot action", Steps: []string{reason}}
		enrichExplain(&p, action, agentCtx.Namespace, targetName)
		e.attachGraphImpact(&p)
		_ = e.audit(p)
		return &p, nil
	}
	if confidence < pol.MinConfidence {
		p := baseProposal(agentCtx, action, targetKind, targetName, confidence, replicas)
		p.Decision = DecisionDenied
		p.Reason = fmt.Sprintf("confidence %.2f below floor %.2f", confidence, pol.MinConfidence)
		p.Risk = "denied"
		p.Plan = PlanBody{Summary: "Confidence gate failed", Steps: []string{p.Reason}}
		enrichExplain(&p, action, agentCtx.Namespace, targetName)
		e.attachGraphImpact(&p)
		_ = e.audit(p)
		return &p, nil
	}

	p := baseProposal(agentCtx, action, targetKind, targetName, confidence, replicas)
	p.Plan = planFor(action, agentCtx.Namespace, targetName, replicas)
	enrichExplain(&p, action, agentCtx.Namespace, targetName)
	e.attachGraphImpact(&p)
	if biased, note := biasActionConfidence(confidence, match, matched); note != "" {
		p.ActionConfidence = biased
		p.LearnNote = note
		p.Why = strings.TrimSpace(p.Why + "; " + note)
	}
	e.attachFleetOutcome(&p, matched)
	p.Decision = DecisionProposed
	p.Risk = riskFor(action)
	p.Reason = "proposeOnly (ADR-0015 default); human apply via CLI or policyAuto"
	if pol.PolicyAuto() {
		p.Decision = DecisionApproved
		p.Reason = "policyAuto: approved under RemediationPolicy; apply requires ApplyProposal / --approve bridge"
		p.Applied = false
	}
	_ = e.audit(p)
	return &p, nil
}

func (e *Engine) audit(p Proposal) error {
	if e.Audit == nil {
		return nil
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Audit.Append(AuditEntry{At: time.Now().UTC(), Proposal: p})
}

// attachGraphImpact appends topology depends_on/allows notes into ExpectedImpact (RT-015).
func (e *Engine) attachGraphImpact(p *Proposal) {
	if e == nil || e.Graph == nil || p == nil {
		return
	}
	note := graph.ImpactNotes(*e.Graph, p.Namespace, p.TargetKind, p.TargetName)
	if note == "" {
		return
	}
	if strings.TrimSpace(p.ExpectedImpact) == "" {
		p.ExpectedImpact = note
		return
	}
	p.ExpectedImpact = strings.TrimSpace(p.ExpectedImpact) + "; " + note
}

const (
	// DefaultFleetTTL caches the Coordinator outcome summary between fetches (RT-022).
	DefaultFleetTTL = 60 * time.Second
	// fleetMinSamples requires this much fleet history before biasing (RT-022).
	fleetMinSamples = 3
	// fleetMaxDelta caps the additive fleet bias well under patterns.MaxBoost (RT-022).
	fleetMaxDelta = 0.05
)

// fleetSummary returns a short-TTL cached Coordinator outcome summary (RT-022).
func (e *Engine) fleetSummary() (coordinator.OutcomeSummary, bool) {
	if e == nil || e.Fleet == nil {
		return coordinator.OutcomeSummary{}, false
	}
	ttl := e.FleetTTL
	if ttl <= 0 {
		ttl = DefaultFleetTTL
	}
	e.mu.Lock()
	if e.fleetCache != nil && time.Since(e.fleetFetched) < ttl {
		sum := *e.fleetCache
		e.mu.Unlock()
		return sum, true
	}
	e.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	sum, err := e.Fleet.Outcomes(ctx)
	if err != nil {
		return coordinator.OutcomeSummary{}, false
	}
	e.mu.Lock()
	e.fleetCache = &sum
	e.fleetFetched = time.Now()
	e.mu.Unlock()
	return sum, true
}

// attachFleetOutcome nudges ActionConfidence from cross-ns fleet outcomes (RT-022).
// Evidence-not-proof (AG-034): applies ONLY when local Learn already matched, caps
// the delta hard, requires a minimum fleet sample, touches ActionConfidence only
// (never the raw confidence gate), and always labels the note as advisory.
func (e *Engine) attachFleetOutcome(p *Proposal, localMatched bool) {
	if e == nil || e.Fleet == nil || p == nil {
		return
	}
	// AG-034 guard: fleet data can only nudge a locally-supported proposal.
	if !localMatched {
		return
	}
	sum, ok := e.fleetSummary()
	if !ok {
		return
	}
	stat, ok := sum.LookupAction(p.ActionID, p.Namespace)
	if !ok || stat.Total < fleetMinSamples {
		return
	}
	// Success ratio in [0,1]; center at 0.5 so a strong track record nudges up,
	// a poor one nudges down, both bounded by fleetMaxDelta.
	ratio := float64(stat.Success) / float64(stat.Total)
	delta := fleetMaxDelta * (ratio*2 - 1)
	base := p.ActionConfidence
	if base <= 0 {
		base = p.Confidence
	}
	p.ActionConfidence = clamp01(base + delta)
	scope := "fleet"
	if stat.Namespace != "" {
		scope = "ns=" + stat.Namespace
	}
	note := fmt.Sprintf("Fleet evidence (not proof): %s success=%d/%d %s Δ=%+.2f — bias only (AG-034/RT-022)",
		p.ActionID, stat.Success, stat.Total, scope, delta)
	if strings.TrimSpace(p.ExpectedImpact) == "" {
		p.ExpectedImpact = note
	} else {
		p.ExpectedImpact = strings.TrimSpace(p.ExpectedImpact) + "; " + note
	}
	if strings.TrimSpace(p.LearnNote) == "" {
		p.LearnNote = note
	} else {
		p.LearnNote = strings.TrimSpace(p.LearnNote) + "; " + note
	}
}

func baseProposal(agentCtx ctxbuild.AgentContext, action, kind, name string, confidence float64, replicas *int32) Proposal {
	id := fmt.Sprintf("ap-%s-%d", action, time.Now().UTC().UnixNano())
	return Proposal{
		APIVersion:       APIVersion,
		Kind:             KindProposal,
		SchemaVersion:    SchemaVersion,
		ID:               id,
		Namespace:        agentCtx.Namespace,
		ActionID:         action,
		Confidence:       confidence,
		ActionConfidence: confidence,
		IncidentID:       agentCtx.Incident.ID,
		TargetKind:       kind,
		TargetName:       name,
		Replicas:         replicas,
		Applied:          false,
		CreatedAt:        time.Now().UTC(),
	}
}

func inList(list []string, v string) bool {
	for _, x := range list {
		if strings.EqualFold(strings.TrimSpace(x), v) {
			return true
		}
	}
	return false
}

// DefaultAuditDir returns ~/.config/kprompt/autopilot.
func DefaultAuditDir() string {
	if d := strings.TrimSpace(os.Getenv("KPROMPT_AUTOPILOT_AUDIT_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".kprompt-autopilot")
	}
	return filepath.Join(home, ".config", "kprompt", "autopilot")
}

// IncidentConfidence picks a confidence hint from alert/context.
func IncidentConfidence(inc incident.Incident, fallback float64) float64 {
	if inc.Confidence > 0 {
		return inc.Confidence
	}
	return fallback
}
