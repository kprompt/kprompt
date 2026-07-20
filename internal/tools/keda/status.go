package keda

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ScaledObjectStatus is a compact view of a KEDA ScaledObject resource.
type ScaledObjectStatus struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	Message   string `json:"message,omitempty"`
	HPAName   string `json:"hpaName,omitempty"`
}

// StatusFromObject extracts ScaledObject status fields from an unstructured object.
func StatusFromObject(obj *unstructured.Unstructured) ScaledObjectStatus {
	if obj == nil {
		return ScaledObjectStatus{}
	}
	st := ScaledObjectStatus{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if st.Namespace == "" {
		st.Namespace = "default"
	}
	hpa, _, _ := unstructured.NestedString(obj.Object, "status", "hpaName")
	st.HPAName = strings.TrimSpace(hpa)

	conds, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		for _, raw := range conds {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			if !strings.EqualFold(typ, "Ready") {
				continue
			}
			status, _ := m["status"].(string)
			msg, _ := m["message"].(string)
			st.Message = strings.TrimSpace(msg)
			switch strings.ToLower(strings.TrimSpace(status)) {
			case "true":
				st.Phase = "Ready"
			case "false":
				st.Phase = "NotReady"
			default:
				st.Phase = "Unknown"
			}
			break
		}
	}
	if st.Phase == "" {
		st.Phase = "Pending"
	}
	return st
}

// Label formats a human-readable ScaledObject status line.
func (s ScaledObjectStatus) Label() string {
	line := fmt.Sprintf("ScaledObject/%s phase=%s", s.Name, s.Phase)
	if s.HPAName != "" {
		line += fmt.Sprintf(" hpa=%s", s.HPAName)
	}
	if s.Message != "" {
		line += fmt.Sprintf(" (%s)", s.Message)
	}
	return line
}
