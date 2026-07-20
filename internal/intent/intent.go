package intent

import "encoding/json"

// Kind classifies a user intent.
type Kind string

const (
	KindDeploy      Kind = "deploy"
	KindInstall     Kind = "install"
	KindUpgrade     Kind = "upgrade"
	KindScale       Kind = "scale"
	KindRollback    Kind = "rollback"
	KindGet         Kind = "get"
	KindExplain     Kind = "explain"
	KindLogs        Kind = "logs"
	KindDescribe    Kind = "describe"
	KindWorkflow    Kind = "workflow"
	KindTekton      Kind = "tekton"
	KindKEDA        Kind = "keda"
	KindIstio       Kind = "istio"
	KindPerformance Kind = "performance"
	KindTrace       Kind = "trace"
	KindDashboard   Kind = "dashboard"
	KindOptimize    Kind = "optimize"
	KindGraph       Kind = "graph"
	KindDelete      Kind = "delete"
	KindPatch       Kind = "patch"
	KindDeny        Kind = "deny"
	KindUnknown     Kind = "unknown"
)

// Intent is the structured result of NL understanding.
type Intent struct {
	Kind       Kind           `json:"kind"`
	Target     Target         `json:"target"`
	Context    string         `json:"context,omitempty"` // kubeconfig context from prompt
	Params     map[string]any `json:"params,omitempty"`
	Confidence float64        `json:"confidence,omitempty"`
	Raw        string         `json:"-"`
}

// Target identifies a Kubernetes object or query scope.
type Target struct {
	Name      string `json:"name,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Kind      string `json:"kind,omitempty"` // Deployment, Pod, ...
}

// SchemaJSON is the versioned JSON schema used for structured LLM output.
const SchemaJSON = `{
  "type": "object",
  "additionalProperties": false,
  "required": ["kind", "target"],
  "properties": {
    "kind": {
      "type": "string",
      "enum": ["deploy", "install", "upgrade", "scale", "rollback", "get", "explain", "logs", "describe", "workflow", "tekton", "keda", "istio", "performance", "trace", "dashboard", "optimize", "graph", "delete", "deny", "unknown"]
    },
    "target": {
      "type": "object",
      "additionalProperties": false,
      "properties": {
        "name": { "type": "string" },
        "namespace": { "type": "string" },
        "kind": { "type": "string" }
      }
    },
    "context": { "type": "string" },
    "params": {
      "type": "object",
      "additionalProperties": true
    },
    "confidence": { "type": "number" }
  }
}`

// ParseStructured validates and unmarshals model JSON into Intent.
func ParseStructured(raw []byte) (Intent, error) {
	var in Intent
	if err := json.Unmarshal(raw, &in); err != nil {
		return Intent{}, err
	}
	if in.Kind == "" {
		in.Kind = KindUnknown
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	return in, nil
}

// Replicas extracts an int replicas param when present.
func (i Intent) Replicas() (int32, bool) {
	v, ok := i.Params["replicas"]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case float64:
		return int32(n), true
	case int:
		return int32(n), true
	case int32:
		return n, true
	case json.Number:
		i64, err := n.Int64()
		if err != nil {
			return 0, false
		}
		return int32(i64), true
	default:
		return 0, false
	}
}
