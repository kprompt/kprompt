package optimize

import (
	"fmt"
	"strings"
	"time"
)

const (
	ScopeCluster   = "cluster"
	ScopeNamespace = "namespace"

	SectionPending = "pending"
	SectionReady   = "ready"
	SectionSkipped = "skipped"

	SeverityInfo = "info"
)

// Request configures a read-only optimize report.
type Request struct {
	Namespace string // empty → cluster-wide scope
	Window    time.Duration
}

// Finding is one stable optimize insight (inventory/idle/rightsizing/HPA fill these later).
type Finding struct {
	Code      string `json:"code"`
	Severity  string `json:"severity"` // info | low | medium | high
	Title     string `json:"title"`
	Message   string `json:"message"`
	Resource  string `json:"resource,omitempty"`
	Namespace string `json:"namespace,omitempty"`
}

// Suggestion is a non-mutating recommendation (T-057 may turn these into optional plans).
type Suggestion struct {
	Code       string `json:"code"`
	Title      string `json:"title"`
	Message    string `json:"message"`
	ActionHint string `json:"actionHint,omitempty"` // scale | patch-resources | hpa | none
}

// SectionStatus tracks whether a report section has real signals yet.
type SectionStatus struct {
	Status  string `json:"status"` // pending | ready | skipped
	Message string `json:"message,omitempty"`
}

// Sections holds the optimize pack subsections (T-053…T-056).
type Sections struct {
	Inventory   SectionStatus `json:"inventory"`
	Idle        SectionStatus `json:"idle"`
	Rightsizing SectionStatus `json:"rightsizing"`
	HPA         SectionStatus `json:"hpa"`
}

// Report is the stable human + JSON contract for `optimize my cluster` (T-052).
type Report struct {
	Type        string       `json:"type"`
	Scope       string       `json:"scope"`
	Namespace   string       `json:"namespace,omitempty"`
	Window      string       `json:"window,omitempty"`
	Summary     string       `json:"summary"`
	Findings    []Finding    `json:"findings"`
	Suggestions []Suggestion `json:"suggestions"`
	Sections    Sections     `json:"sections"`
}

// BuildScaffold returns a read-only optimize report shell.
// Inventory / idle / rightsizing / HPA signals land in T-053…T-056; this task freezes the shape only.
func BuildScaffold(req Request) Report {
	ns := strings.TrimSpace(req.Namespace)
	scope := ScopeCluster
	if ns != "" {
		scope = ScopeNamespace
	}
	window := req.Window
	if window <= 0 {
		window = 1 * time.Hour
	}
	windowLabel := formatWindow(window)

	summary := "Optimize report scaffold (read-only). Workload inventory, idle detection, rightsizing, and HPA hints will fill in as those signals ship — no mutations in this pass."
	if scope == ScopeNamespace {
		summary = fmt.Sprintf(
			"Optimize report scaffold for namespace %q (read-only). Inventory and Prometheus-backed sections land in follow-up tasks — no mutations in this pass.",
			ns,
		)
	}

	return Report{
		Type:      "optimize",
		Scope:     scope,
		Namespace: ns,
		Window:    windowLabel,
		Summary:   summary,
		Findings: []Finding{{
			Code:     "optimize.scaffold",
			Severity: SeverityInfo,
			Title:    "Read-only optimize contract",
			Message:  "This report never applies changes. Optional fix plans (scale/patch) require explicit approval in a later task.",
		}},
		Suggestions: nil,
		Sections: Sections{
			Inventory: SectionStatus{
				Status:  SectionPending,
				Message: "Workload inventory (replicas, requests/limits) — T-053",
			},
			Idle: SectionStatus{
				Status:  SectionPending,
				Message: "Idle / underutilized detection via Prometheus — T-054",
			},
			Rightsizing: SectionStatus{
				Status:  SectionPending,
				Message: "CPU/memory rightsizing suggestions — T-055",
			},
			HPA: SectionStatus{
				Status:  SectionPending,
				Message: "HPA / replica hint narration — T-056",
			},
		},
	}
}

func formatWindow(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return d.String()
}
