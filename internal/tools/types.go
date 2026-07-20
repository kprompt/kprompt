package tools

// ID identifies an integration backend.
type ID string

const (
	IDKubernetes    ID = "kubernetes"
	IDHelm          ID = "helm"
	IDArgoWorkflows ID = "argo-workflows"
	IDTekton        ID = "tekton"
	IDKEDA          ID = "keda"
	IDIstio         ID = "istio"
	IDCrossplane    ID = "crossplane"
	IDPrometheus    ID = "prometheus"
	IDGrafana       ID = "grafana"
	IDOpenTelemetry ID = "opentelemetry"
)

// Capability is a coarse operation class a tool may support (future planners).
type Capability string

const (
	CapQuery   Capability = "query"
	CapInstall Capability = "install"
	CapUpgrade Capability = "upgrade"
	CapSubmit  Capability = "submit"
	CapMutate  Capability = "mutate"
)

// Status is the outcome of capability detection.
type Status string

const (
	StatusAvailable   Status = "available"
	StatusConfigured  Status = "configured" // URL set but probe failed or not run
	StatusUnavailable Status = "unavailable"
	StatusDisabled    Status = "disabled" // opted out in config
)

// Result is the detect output for one tool.
type Result struct {
	ID           ID
	Name         string
	Status       Status
	Detail       string
	Hint         string
	Capabilities []Capability
}

// Available reports whether the tool can be used for planning right now.
func (r Result) Available() bool {
	return r.Status == StatusAvailable || r.Status == StatusConfigured
}
