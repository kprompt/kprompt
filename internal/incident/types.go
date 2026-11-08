// Package incident defines the shared investigation / agent alert contracts (AG-002, S-001, AG-022).
//
// CLI investigate/why/timeline/impact/audit and the in-cluster Observe agent both emit these shapes.
// Notifiers (Slack, webhook) serialize AgentAlert — never free-form chat as the artifact of truth.
// Namespace Agent adds InvestigationReport (schemaVersion 2) for causal RCA + Coordinator handoff (ADR-0016).
// Optional suggested fixes remain PlanResult-shaped and require approval (ADR-0003); Observe Mode never mutates.
package incident

import (
	"fmt"
	"strings"
	"time"
)

const (
	APIVersion    = "kprompt.io/v1"
	SchemaVersion = "1"

	KindIncident      = "Incident"
	KindAgentAlert    = "AgentAlert"
	KindInvestigation = "Investigation"

	// Severity levels align with optimize findings + a critical tier for paging.
	SeverityInfo     = "info"
	SeverityLow      = "low"
	SeverityMedium   = "medium"
	SeverityHigh     = "high"
	SeverityCritical = "critical"

	StatusOpen      = "open"
	StatusMitigated = "mitigated"
	StatusResolved  = "resolved"
	StatusClosed    = "closed"

	AlertFired     = "fired"
	AlertUpdated   = "updated"
	AlertRecovered = "recovered"

	// Evidence kinds.
	EvidenceEvent  = "event"
	EvidenceLog    = "log"
	EvidenceObject = "object"
	EvidenceMetric = "metric"
	EvidenceTrace  = "trace"
	EvidenceGitOps = "gitops" // AG-035 Argo/Flux sync + deploy history
)

// ResourceRef is a Kubernetes object identity (JSON-friendly ObjectRef).
type ResourceRef struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
}

// EvidenceRef points at supporting cluster signal without embedding huge payloads.
type EvidenceRef struct {
	Type      string       `json:"type"` // event | log | object | metric | trace
	Resource  *ResourceRef `json:"resource,omitempty"`
	Reason    string       `json:"reason,omitempty"`
	Message   string       `json:"message,omitempty"`
	Timestamp *time.Time   `json:"timestamp,omitempty"`
	Source    string       `json:"source,omitempty"` // kubernetes | prometheus | otel | agent
	// URI is an optional locator (e.g. log window hint); keep short — no cluster dumps.
	URI string `json:"uri,omitempty"`
}

// Finding is one RCA / investigation insight (shared by CLI investigate and agent analysis).
type Finding struct {
	Code      string        `json:"code"`
	Severity  string        `json:"severity"` // info | low | medium | high | critical
	Title     string        `json:"title"`
	Message   string        `json:"message"`
	Evidence  []EvidenceRef `json:"evidence,omitempty"`
	Namespace string        `json:"namespace,omitempty"`
}

// Incident is a correlated event window for one workload/namespace (agent incident builder).
type Incident struct {
	APIVersion     string     `json:"apiVersion"`
	Kind           string     `json:"kind"`
	SchemaVersion  string     `json:"schemaVersion"`
	ID             string     `json:"id"`
	Namespace      string     `json:"namespace"`
	ClusterContext string     `json:"cluster_context,omitempty"`
	Status         string     `json:"status"` // open | mitigated | resolved | closed
	StartedAt      time.Time  `json:"startedAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	ClosedAt       *time.Time `json:"closedAt,omitempty"`

	PrimaryResource *ResourceRef  `json:"primaryResource,omitempty"`
	Affected        []ResourceRef `json:"affected,omitempty"`
	Evidence        []EvidenceRef `json:"evidence,omitempty"`
	Findings        []Finding     `json:"findings,omitempty"`

	Summary         string  `json:"summary,omitempty"`
	RootCause       string  `json:"rootCause,omitempty"`
	Confidence      float64 `json:"confidence,omitempty"` // 0..1
	Recommendation  string  `json:"recommendation,omitempty"`
	Severity        string  `json:"severity,omitempty"`
	NotifierThread  string  `json:"notifierThread,omitempty"` // e.g. Slack thread ts
	HealthScoreHint *int    `json:"healthScoreHint,omitempty"`

	// RT-001: last Autopilot/CLI remediation outcome stamped after apply+verify.
	LastApplyOutcome string `json:"lastApplyOutcome,omitempty"` // apply_success | apply_failed | apply_partial
	LastVerifyStatus string `json:"lastVerifyStatus,omitempty"` // ok | pending | failed | skipped
	LastActionID     string `json:"lastActionId,omitempty"`
}

// AgentAlert is the outbound notify artifact (Slack / webhook JSON body).
type AgentAlert struct {
	APIVersion     string        `json:"apiVersion"`
	Kind           string        `json:"kind"`
	SchemaVersion  string        `json:"schemaVersion"`
	IncidentID     string        `json:"incidentId"`
	Namespace      string        `json:"namespace"`
	ClusterContext string        `json:"cluster_context,omitempty"`
	Status         string        `json:"status"` // fired | updated | recovered
	Severity       string        `json:"severity"`
	Confidence     float64       `json:"confidence"` // 0..1
	Summary        string        `json:"summary"`
	RootCause      string        `json:"rootCause,omitempty"`
	Recommendation string        `json:"recommendation,omitempty"`
	Affected       []ResourceRef `json:"affected,omitempty"`
	Evidence       []EvidenceRef `json:"evidence,omitempty"`
	CreatedAt      time.Time     `json:"createdAt"`

	// RT-017 · RT-018: optional AutopilotProposal bridge (propose path; never silent apply).
	ProposalID     string `json:"proposalId,omitempty"`
	ProposalAction string `json:"proposalAction,omitempty"`
	ProposalRisk   string `json:"proposalRisk,omitempty"` // low|medium|high|denied — same words as CLI
	ProposalHint   string `json:"proposalHint,omitempty"` // CLI: agent proposals apply … --approve
	// IC-006: compact impact line from AutopilotProposal.expectedImpact (not full blastRadius).
	ProposalImpact string `json:"proposalImpact,omitempty"`
}

// Investigation is the CLI-facing intelligence document for investigate / why / timeline / impact.
// Suggested fixes are not applied here — callers may attach a PlanResult separately under approval.
type Investigation struct {
	APIVersion     string        `json:"apiVersion"`
	Kind           string        `json:"kind"`
	SchemaVersion  string        `json:"schemaVersion"`
	Prompt         string        `json:"prompt,omitempty"`
	Namespace      string        `json:"namespace,omitempty"`
	ClusterContext string        `json:"cluster_context,omitempty"`
	Target         *ResourceRef  `json:"target,omitempty"`
	Summary        string        `json:"summary"`
	RootCause      string        `json:"rootCause,omitempty"`
	Confidence     float64       `json:"confidence,omitempty"` // 0..1
	Findings       []Finding     `json:"findings"`
	Evidence       []EvidenceRef `json:"evidence,omitempty"`
	Timeline       []EvidenceRef `json:"timeline,omitempty"` // ordered chronology (S-004)
	Degraded       []string      `json:"degraded,omitempty"` // e.g. "prometheus", "otel", "mesh"
	// SuggestedPlanHint describes a fix path; never auto-applied (use PlanResult + approve).
	SuggestedPlanHint string `json:"suggestedPlanHint,omitempty"`
}

// NewIncident returns a schema-stamped open incident shell.
func NewIncident(id, namespace string, started time.Time) Incident {
	if started.IsZero() {
		started = time.Now().UTC()
	}
	return Incident{
		APIVersion:    APIVersion,
		Kind:          KindIncident,
		SchemaVersion: SchemaVersion,
		ID:            strings.TrimSpace(id),
		Namespace:     strings.TrimSpace(namespace),
		Status:        StatusOpen,
		StartedAt:     started.UTC(),
		UpdatedAt:     started.UTC(),
	}
}

// NewAgentAlert builds a notify payload from an analyzed incident.
func NewAgentAlert(inc Incident, status string, at time.Time) AgentAlert {
	if at.IsZero() {
		at = time.Now().UTC()
	}
	sev := strings.TrimSpace(inc.Severity)
	if sev == "" {
		sev = SeverityMedium
	}
	st := strings.TrimSpace(status)
	if st == "" {
		st = AlertFired
	}
	return AgentAlert{
		APIVersion:     APIVersion,
		Kind:           KindAgentAlert,
		SchemaVersion:  SchemaVersion,
		IncidentID:     inc.ID,
		Namespace:      inc.Namespace,
		ClusterContext: inc.ClusterContext,
		Status:         st,
		Severity:       sev,
		Confidence:     inc.Confidence,
		Summary:        inc.Summary,
		RootCause:      inc.RootCause,
		Recommendation: inc.Recommendation,
		Affected:       append([]ResourceRef(nil), inc.Affected...),
		Evidence:       append([]EvidenceRef(nil), inc.Evidence...),
		CreatedAt:      at.UTC(),
	}
}

// NewInvestigation returns a schema-stamped investigation shell.
func NewInvestigation(prompt, namespace string) Investigation {
	return Investigation{
		APIVersion:    APIVersion,
		Kind:          KindInvestigation,
		SchemaVersion: SchemaVersion,
		Prompt:        strings.TrimSpace(prompt),
		Namespace:     strings.TrimSpace(namespace),
		Findings:      []Finding{},
	}
}

// ValidateIncident checks required fields for persistence / notify gates.
func ValidateIncident(inc Incident) error {
	if strings.TrimSpace(inc.APIVersion) == "" || strings.TrimSpace(inc.Kind) == "" {
		return fmt.Errorf("incident: apiVersion and kind are required")
	}
	if strings.TrimSpace(inc.ID) == "" {
		return fmt.Errorf("incident: id is required")
	}
	if strings.TrimSpace(inc.Namespace) == "" {
		return fmt.Errorf("incident: namespace is required")
	}
	if !validStatus(inc.Status) {
		return fmt.Errorf("incident: invalid status %q", inc.Status)
	}
	if inc.Confidence < 0 || inc.Confidence > 1 {
		return fmt.Errorf("incident: confidence must be in [0,1], got %v", inc.Confidence)
	}
	if sev := strings.TrimSpace(inc.Severity); sev != "" && !validSeverity(sev) {
		return fmt.Errorf("incident: invalid severity %q", sev)
	}
	return nil
}

// ValidateAgentAlert checks notify payload before Slack/webhook send.
func ValidateAgentAlert(a AgentAlert) error {
	if strings.TrimSpace(a.APIVersion) == "" || strings.TrimSpace(a.Kind) == "" {
		return fmt.Errorf("agentAlert: apiVersion and kind are required")
	}
	if strings.TrimSpace(a.IncidentID) == "" {
		return fmt.Errorf("agentAlert: incidentId is required")
	}
	if strings.TrimSpace(a.Namespace) == "" {
		return fmt.Errorf("agentAlert: namespace is required")
	}
	if strings.TrimSpace(a.Summary) == "" {
		return fmt.Errorf("agentAlert: summary is required")
	}
	if !validAlertStatus(a.Status) {
		return fmt.Errorf("agentAlert: invalid status %q", a.Status)
	}
	if !validSeverity(a.Severity) {
		return fmt.Errorf("agentAlert: invalid severity %q", a.Severity)
	}
	if a.Confidence < 0 || a.Confidence > 1 {
		return fmt.Errorf("agentAlert: confidence must be in [0,1], got %v", a.Confidence)
	}
	return nil
}

// ValidateInvestigation checks CLI RCA document shape.
func ValidateInvestigation(inv Investigation) error {
	if strings.TrimSpace(inv.APIVersion) == "" || strings.TrimSpace(inv.Kind) == "" {
		return fmt.Errorf("investigation: apiVersion and kind are required")
	}
	if strings.TrimSpace(inv.Summary) == "" {
		return fmt.Errorf("investigation: summary is required")
	}
	if inv.Confidence < 0 || inv.Confidence > 1 {
		return fmt.Errorf("investigation: confidence must be in [0,1], got %v", inv.Confidence)
	}
	for i, f := range inv.Findings {
		if strings.TrimSpace(f.Code) == "" || strings.TrimSpace(f.Title) == "" {
			return fmt.Errorf("investigation: findings[%d] needs code and title", i)
		}
		if sev := strings.TrimSpace(f.Severity); sev != "" && !validSeverity(sev) {
			return fmt.Errorf("investigation: findings[%d] invalid severity %q", i, sev)
		}
	}
	return nil
}

// MeetsAlertGate returns true when severity/confidence clear configured floors.
func MeetsAlertGate(a AgentAlert, minSeverity string, minConfidence float64) bool {
	if a.Confidence < minConfidence {
		return false
	}
	return severityRank(a.Severity) >= severityRank(minSeverity)
}

func validSeverity(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SeverityInfo, SeverityLow, SeverityMedium, SeverityHigh, SeverityCritical:
		return true
	default:
		return false
	}
}

func validStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case StatusOpen, StatusMitigated, StatusResolved, StatusClosed:
		return true
	default:
		return false
	}
}

func validAlertStatus(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case AlertFired, AlertUpdated, AlertRecovered:
		return true
	default:
		return false
	}
}

func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case SeverityInfo:
		return 1
	case SeverityLow:
		return 2
	case SeverityMedium:
		return 3
	case SeverityHigh:
		return 4
	case SeverityCritical:
		return 5
	default:
		return 0
	}
}

// DefaultMinSeverity is the Observe Mode alert floor (warning ≈ medium).
func DefaultMinSeverity() string { return SeverityMedium }

// DefaultMinConfidence is the Observe Mode confidence floor.
func DefaultMinConfidence() float64 { return 0.7 }
