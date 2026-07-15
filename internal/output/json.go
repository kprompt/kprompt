package output

import (
	"encoding/json"
	"io"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

const (
	APIVersion    = "kprompt.io/v1"
	KindPlanResult = "PlanResult"
	SchemaVersion = "1"
)

// PlanResult is the stable CI-facing JSON document.
type PlanResult struct {
	APIVersion    string          `json:"apiVersion"`
	Kind          string          `json:"kind"`
	SchemaVersion string          `json:"schemaVersion"`
	Prompt        string          `json:"prompt"`
	Plan          PlanPayload     `json:"plan"`
	Risk          RiskPayload     `json:"risk"`
	Applied       bool            `json:"applied"`
	Result        json.RawMessage `json:"result,omitempty"`
}

// PlanPayload is the reviewable plan without manifests/secrets.
type PlanPayload struct {
	Intent           string          `json:"intent"`
	Summary          string          `json:"summary"`
	RequiresApproval bool            `json:"requiresApproval"`
	Namespace        string          `json:"namespace,omitempty"`
	Context          string          `json:"context,omitempty"`
	Actions          []ActionPayload `json:"actions"`
}

// ActionPayload is one planned step (no YAML manifests).
type ActionPayload struct {
	Op        string `json:"op"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Replicas  *int32 `json:"replicas,omitempty"`
	Revision  *int64 `json:"revision,omitempty"`
	Diff      string `json:"diff,omitempty"`
}

// RiskPayload mirrors safety evaluation for CI gates.
type RiskPayload struct {
	Level   string `json:"level"`
	Denied  bool   `json:"denied"`
	Message string `json:"message,omitempty"`
}

// FromPlan builds a PlanResult from the in-memory plan.
func FromPlan(prompt, kubeContext string, plan planner.ExecutionPlan, risk safety.Result, applied bool) PlanResult {
	ns := plan.Intent.Target.Namespace
	actions := make([]ActionPayload, 0, len(plan.Actions))
	for _, a := range plan.Actions {
		if ns == "" && a.Object.Namespace != "" {
			ns = a.Object.Namespace
		}
		actions = append(actions, ActionPayload{
			Op:        string(a.Op),
			Kind:      a.Object.Kind,
			Name:      a.Object.Name,
			Namespace: a.Object.Namespace,
			Replicas:  a.Replicas,
			Revision:  a.Revision,
			Diff:      a.Diff,
		})
	}
	return PlanResult{
		APIVersion:    APIVersion,
		Kind:          KindPlanResult,
		SchemaVersion: SchemaVersion,
		Prompt:        prompt,
		Plan: PlanPayload{
			Intent:           string(plan.Intent.Kind),
			Summary:          plan.Summary,
			RequiresApproval: plan.RequiresApproval,
			Namespace:        ns,
			Context:          kubeContext,
			Actions:          actions,
		},
		Risk: RiskPayload{
			Level:   string(risk.Risk),
			Denied:  risk.Denied,
			Message: risk.Message,
		},
		Applied: applied,
	}
}

// WithQueryResult attaches a tabular get/list payload.
func (r PlanResult) WithQueryResult(res cluster.Result) PlanResult {
	payload := map[string]any{
		"type":    "query",
		"kind":    res.Kind,
		"headers": res.Headers,
		"rows":    queryRows(res),
	}
	raw, _ := json.Marshal(payload)
	r.Result = raw
	return r
}

// WithExplainResult attaches an explain-lite payload.
func (r PlanResult) WithExplainResult(rep cluster.ExplainReport) PlanResult {
	payload := map[string]any{
		"type":      "explain",
		"target":    rep.Target,
		"namespace": rep.Namespace,
		"kind":      rep.Kind,
		"status":    rep.Status,
		"summary":   rep.Summary,
		"findings":  rep.Findings,
		"events":    rep.Events,
	}
	raw, _ := json.Marshal(payload)
	r.Result = raw
	return r
}

// WithDescribeResult attaches a compact describe payload.
func (r PlanResult) WithDescribeResult(rep cluster.DescribeReport) PlanResult {
	payload := map[string]any{
		"type":      "describe",
		"kind":      rep.Kind,
		"name":      rep.Name,
		"namespace": rep.Namespace,
		"status":    rep.Status,
		"lines":     rep.Lines,
	}
	raw, _ := json.Marshal(payload)
	r.Result = raw
	return r
}

// WithLogsResult attaches a log-tail payload (body may be truncated for CI).
func (r PlanResult) WithLogsResult(res cluster.LogsResult) PlanResult {
	body := res.Body
	const max = 32 * 1024
	if len(body) > max {
		body = body[:max] + "\n…(truncated)"
	}
	payload := map[string]any{
		"type":      "logs",
		"pod":       res.Pod,
		"namespace": res.Namespace,
		"container": res.Container,
		"tail":      res.Tail,
		"body":      body,
	}
	raw, _ := json.Marshal(payload)
	r.Result = raw
	return r
}

// Encode writes compact JSON plus a trailing newline.
func Encode(w io.Writer, r PlanResult) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(r)
}

func queryRows(res cluster.Result) []map[string]string {
	out := make([]map[string]string, 0, len(res.Rows))
	for _, row := range res.Rows {
		m := map[string]string{
			"namespace": row.Namespace,
			"name":      row.Name,
			"ready":     row.Ready,
			"status":    row.Status,
		}
		if row.Extra != "" {
			m["extra"] = row.Extra
		}
		out = append(out, m)
	}
	return out
}
