package tools

import (
	"github.com/kprompt/kprompt/internal/tools/argo"
	"github.com/kprompt/kprompt/internal/tools/tekton"
)

// MissingHint returns an actionable message when a backend is not available.
func MissingHint(id ID) string {
	switch id {
	case IDHelm:
		return "Helm is not available. Install Helm (https://helm.sh/docs/intro/install/) or use the Kubernetes shortcut: kprompt \"deploy redis\""
	case IDArgoWorkflows:
		return argo.InstallHint()
	case IDTekton:
		return tekton.InstallHint()
	case IDPrometheus:
		return "Prometheus is not configured. Set KPROMPT_PROMETHEUS_URL or tools.prometheus.url in ~/.kprompt/config.yaml"
	case IDGrafana:
		return "Grafana is not configured. Set KPROMPT_GRAFANA_URL (and KPROMPT_GRAFANA_API_KEY when the API requires it)"
	case IDOpenTelemetry:
		return "Trace backend is not configured. Set KPROMPT_OTEL_ENDPOINT to a Jaeger/Tempo query URL and KPROMPT_OTEL_BACKEND=jaeger|tempo (or tools.otel.* in ~/.kprompt/config.yaml)"
	case IDKubernetes:
		return "Kubernetes is not reachable. Check kubeconfig and context (kubectl config current-context)."
	default:
		return "Requested tool integration is not available."
	}
}
