package crossplane

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ClaimStatus is a compact view of a Crossplane claim resource.
type ClaimStatus struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Phase     string `json:"phase"`
	Synced    string `json:"synced,omitempty"`
	Ready     string `json:"ready,omitempty"`
	Message   string `json:"message,omitempty"`
}

// StatusFromObject extracts claim status fields from an unstructured object.
func StatusFromObject(obj *unstructured.Unstructured) ClaimStatus {
	if obj == nil {
		return ClaimStatus{}
	}
	st := ClaimStatus{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
		Kind:      obj.GetKind(),
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
			status, _ := m["status"].(string)
			msg, _ := m["message"].(string)
			switch strings.ToLower(strings.TrimSpace(typ)) {
			case "ready":
				st.Ready = strings.TrimSpace(status)
				if msg != "" {
					st.Message = strings.TrimSpace(msg)
				}
			case "synced":
				st.Synced = strings.TrimSpace(status)
				if st.Message == "" && msg != "" {
					st.Message = strings.TrimSpace(msg)
				}
			}
		}
	}
	switch {
	case strings.EqualFold(st.Ready, "True"):
		st.Phase = "Ready"
	case strings.EqualFold(st.Synced, "True") && st.Ready == "":
		st.Phase = "Synced"
	case st.Ready != "" || st.Synced != "":
		st.Phase = "Pending"
	default:
		st.Phase = "Pending"
	}
	return st
}

// Label formats a human-readable claim status line.
func (s ClaimStatus) Label() string {
	kind := s.Kind
	if kind == "" {
		kind = "Claim"
	}
	line := fmt.Sprintf("%s/%s phase=%s", kind, s.Name, s.Phase)
	if s.Synced != "" {
		line += fmt.Sprintf(" synced=%s", s.Synced)
	}
	if s.Ready != "" {
		line += fmt.Sprintf(" ready=%s", s.Ready)
	}
	if s.Message != "" {
		line += fmt.Sprintf(" (%s)", s.Message)
	}
	return line
}
