package tools

import (
	"fmt"

	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
)

// NewGrafanaClient builds the dashboard adapter from resolved tool settings.
func NewGrafanaClient(
	settings Settings,
	options ...toolgrafana.Option,
) (*toolgrafana.Client, error) {
	if !settings.GrafanaEnabled {
		return nil, fmt.Errorf("Grafana integration is disabled")
	}
	if settings.GrafanaURL == "" {
		return nil, fmt.Errorf("%s", MissingHint(IDGrafana))
	}
	return toolgrafana.New(settings.GrafanaURL, settings.GrafanaAPIKey, options...)
}
