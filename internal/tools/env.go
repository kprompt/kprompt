package tools

import (
	"os"
	"strings"

	"github.com/kprompt/kprompt/internal/config"
)

const (
	EnvPrometheusURL   = "KPROMPT_PROMETHEUS_URL"
	EnvGrafanaURL      = "KPROMPT_GRAFANA_URL"
	EnvGrafanaAPIKey   = "KPROMPT_GRAFANA_API_KEY"
	EnvOTelEndpoint    = "KPROMPT_OTEL_ENDPOINT"
	EnvOTelBackend     = "KPROMPT_OTEL_BACKEND"
	EnvHelmEnabled     = "KPROMPT_HELM_ENABLED"
	EnvPrometheusOn    = "KPROMPT_PROMETHEUS_ENABLED"
	EnvGrafanaOn       = "KPROMPT_GRAFANA_ENABLED"
	EnvOTelOn          = "KPROMPT_OTEL_ENABLED"
	EnvArgoWorkflowsOn = "KPROMPT_ARGO_WORKFLOWS_ENABLED"
	EnvTektonOn        = "KPROMPT_TEKTON_ENABLED"
)

// Settings merges ~/.kprompt/config.yaml tools section with env overrides.
type Settings struct {
	HelmEnabled       bool
	ArgoEnabled       bool
	TektonEnabled     bool
	PrometheusEnabled bool
	GrafanaEnabled    bool
	OTelEnabled       bool
	PrometheusURL     string
	GrafanaURL        string
	GrafanaAPIKey     string
	OTelEndpoint      string
	OTelBackend       string
}

// LoadSettings reads tool-related config and environment.
func LoadSettings(file config.File) Settings {
	s := Settings{
		HelmEnabled:       toolEnabled(file.Tools.Helm.Enabled, EnvHelmEnabled, true),
		ArgoEnabled:       toolEnabled(file.Tools.ArgoWorkflows.Enabled, EnvArgoWorkflowsOn, true),
		TektonEnabled:     toolEnabled(file.Tools.Tekton.Enabled, EnvTektonOn, true),
		PrometheusEnabled: toolEnabled(file.Tools.Prometheus.Enabled, EnvPrometheusOn, true),
		GrafanaEnabled:    toolEnabled(file.Tools.Grafana.Enabled, EnvGrafanaOn, true),
		OTelEnabled:       toolEnabled(file.Tools.OTel.Enabled, EnvOTelOn, true),
		PrometheusURL:     firstNonEmpty(os.Getenv(EnvPrometheusURL), file.Tools.Prometheus.URL),
		GrafanaURL:        firstNonEmpty(os.Getenv(EnvGrafanaURL), file.Tools.Grafana.URL),
		GrafanaAPIKey:     strings.TrimSpace(os.Getenv(EnvGrafanaAPIKey)),
		OTelEndpoint:      firstNonEmpty(os.Getenv(EnvOTelEndpoint), file.Tools.OTel.Endpoint),
		OTelBackend:       firstNonEmpty(os.Getenv(EnvOTelBackend), file.Tools.OTel.Backend, "auto"),
	}
	return s
}

func toolEnabled(cfg *bool, envKey string, defaultOn bool) bool {
	if v := strings.TrimSpace(os.Getenv(envKey)); v != "" {
		switch strings.ToLower(v) {
		case "0", "false", "no", "off":
			return false
		case "1", "true", "yes", "on":
			return true
		}
	}
	if cfg != nil {
		return *cfg
	}
	return defaultOn
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimRight(strings.TrimSpace(v), "/")
		}
	}
	return ""
}
