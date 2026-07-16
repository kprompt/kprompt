package argo

import (
	"fmt"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// WorkflowStatus is a compact view of an Argo Workflow resource.
type WorkflowStatus struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Phase      string `json:"phase"`
	Message    string `json:"message,omitempty"`
	StartedAt  string `json:"startedAt,omitempty"`
	FinishedAt string `json:"finishedAt,omitempty"`
}

// StatusFromObject extracts workflow status fields from an unstructured Workflow.
func StatusFromObject(obj *unstructured.Unstructured) WorkflowStatus {
	if obj == nil {
		return WorkflowStatus{}
	}
	st := WorkflowStatus{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if st.Namespace == "" {
		st.Namespace = "default"
	}
	phase, _, _ := unstructured.NestedString(obj.Object, "status", "phase")
	st.Phase = strings.TrimSpace(phase)
	if st.Phase == "" {
		st.Phase = "Pending"
	}
	msg, _, _ := unstructured.NestedString(obj.Object, "status", "message")
	st.Message = strings.TrimSpace(msg)
	st.StartedAt = nestedTimeString(obj.Object, "status", "startedAt")
	st.FinishedAt = nestedTimeString(obj.Object, "status", "finishedAt")
	return st
}

// IsTerminalPhase reports whether the workflow has finished running.
func IsTerminalPhase(phase string) bool {
	switch strings.TrimSpace(phase) {
	case "Succeeded", "Failed", "Error":
		return true
	default:
		return false
	}
}

// Label formats a human-readable workflow status line.
func (s WorkflowStatus) Label() string {
	line := fmt.Sprintf("Workflow/%s phase=%s", s.Name, s.Phase)
	if s.Message != "" {
		line += fmt.Sprintf(" (%s)", s.Message)
	}
	return line
}

func nestedTimeString(obj map[string]any, fields ...string) string {
	v, found, err := unstructured.NestedFieldCopy(obj, fields...)
	if err != nil || !found || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case time.Time:
		if t.IsZero() {
			return ""
		}
		return t.UTC().Format(time.RFC3339)
	case map[string]any:
		if s, ok := t["time"].(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return strings.TrimSpace(fmt.Sprint(v))
}
