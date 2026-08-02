// Package v1 defines the KpromptAgent API (AG-013).
//
// CRD group: kprompt.ai/v1. Observe Mode only — Autopilot requires a later ADR.
package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	Group   = "kprompt.ai"
	Version = "v1"
	Kind    = "KpromptAgent"

	ModeObserve = "Observe"
)

// KpromptAgent is the Schema for the kpromptagents API.
type KpromptAgent struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   KpromptAgentSpec   `json:"spec,omitempty"`
	Status KpromptAgentStatus `json:"status,omitempty"`
}

// KpromptAgentSpec is the desired Observe agent configuration.
type KpromptAgentSpec struct {
	// Mode must be Observe for V1 (ADR-0013).
	Mode string `json:"mode"`
	// Namespace to watch; empty means the CR's namespace.
	Namespace string `json:"namespace,omitempty"`
	// Watches lists resources to watch (empty → pods + events).
	Watches   []string     `json:"watches,omitempty"`
	LLM       LLMSpec      `json:"llm,omitempty"`
	Notify    NotifySpec   `json:"notify,omitempty"`
	Severity  SeveritySpec `json:"severity,omitempty"`
	FetchLogs *bool        `json:"fetchLogs,omitempty"`
	Health    *bool        `json:"health,omitempty"`
	SecretRef *SecretRef   `json:"secretRef,omitempty"`
	Image     *ImageSpec   `json:"image,omitempty"`
}

// LLMSpec selects the analyzer backend.
type LLMSpec struct {
	Provider  string `json:"provider,omitempty"`
	Model     string `json:"model,omitempty"`
	Heuristic bool   `json:"heuristic,omitempty"`
}

// NotifySpec toggles notifiers (credentials via Secret).
type NotifySpec struct {
	Slack   bool `json:"slack,omitempty"`
	Webhook bool `json:"webhook,omitempty"`
	Discord bool `json:"discord,omitempty"`
}

// SeveritySpec configures the alert gate.
type SeveritySpec struct {
	Minimum       string  `json:"minimum,omitempty"`
	MinConfidence float64 `json:"minConfidence,omitempty"`
}

// SecretRef points at a Secret with LLM / Slack / webhook keys.
type SecretRef struct {
	Name string `json:"name"`
}

// ImageSpec optionally overrides the agent image (Operator).
type ImageSpec struct {
	Repository string `json:"repository,omitempty"`
	Tag        string `json:"tag,omitempty"`
}

// KpromptAgentStatus is the observed state.
type KpromptAgentStatus struct {
	ObservedGeneration int64              `json:"observedGeneration,omitempty"`
	HealthScore        *int               `json:"healthScore,omitempty"`
	HealthTrend        string             `json:"healthTrend,omitempty"`
	LastAlert          *LastAlertStatus   `json:"lastAlert,omitempty"`
	OpenIncidents      int                `json:"openIncidents,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
}

// LastAlertStatus summarizes the most recent gated AgentAlert.
type LastAlertStatus struct {
	IncidentID string       `json:"incidentId,omitempty"`
	Severity   string       `json:"severity,omitempty"`
	Summary    string       `json:"summary,omitempty"`
	Confidence float64      `json:"confidence,omitempty"`
	Status     string       `json:"status,omitempty"`
	At         *metav1.Time `json:"at,omitempty"`
}

// KpromptAgentList contains a list of KpromptAgent.
type KpromptAgentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []KpromptAgent `json:"items"`
}

// DefaultMode returns Observe when mode is empty.
func DefaultMode(mode string) string {
	if mode == "" {
		return ModeObserve
	}
	return mode
}
