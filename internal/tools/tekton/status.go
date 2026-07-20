package tekton

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// PipelineRunStatus is a compact view of a Tekton PipelineRun resource.
type PipelineRunStatus struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	Message   string `json:"message,omitempty"`
}

// StatusFromObject extracts PipelineRun status fields from an unstructured object.
func StatusFromObject(obj *unstructured.Unstructured) PipelineRunStatus {
	if obj == nil {
		return PipelineRunStatus{}
	}
	st := PipelineRunStatus{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if st.Namespace == "" {
		st.Namespace = "default"
	}
	conds, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		for _, raw := range conds {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			if !strings.EqualFold(typ, "Succeeded") {
				continue
			}
			status, _ := m["status"].(string)
			reason, _ := m["reason"].(string)
			msg, _ := m["message"].(string)
			st.Message = strings.TrimSpace(msg)
			switch strings.ToLower(strings.TrimSpace(status)) {
			case "true":
				st.Phase = "Succeeded"
			case "false":
				st.Phase = "Failed"
				if reason != "" {
					st.Phase = reason
				}
			default:
				st.Phase = "Running"
				if reason != "" {
					st.Phase = reason
				}
			}
			break
		}
	}
	if st.Phase == "" {
		st.Phase = "Pending"
	}
	return st
}

// Label formats a human-readable PipelineRun status line.
func (s PipelineRunStatus) Label() string {
	line := fmt.Sprintf("PipelineRun/%s phase=%s", s.Name, s.Phase)
	if s.Message != "" {
		line += fmt.Sprintf(" (%s)", s.Message)
	}
	return line
}
