package tools

import (
	"fmt"

	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

// NewPrometheusClient builds the query adapter from resolved tool settings.
func NewPrometheusClient(
	settings Settings,
	options ...toolprometheus.Option,
) (*toolprometheus.Client, error) {
	if !settings.PrometheusEnabled {
		return nil, fmt.Errorf("Prometheus integration is disabled")
	}
	if settings.PrometheusURL == "" {
		return nil, fmt.Errorf("%s", MissingHint(IDPrometheus))
	}
	return toolprometheus.New(settings.PrometheusURL, options...)
}
