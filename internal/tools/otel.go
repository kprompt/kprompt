package tools

import (
	"fmt"
	"strings"

	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
)

// NewOTelClient builds the trace query adapter from resolved tool settings.
func NewOTelClient(settings Settings, options ...toolotel.Option) (*toolotel.Client, error) {
	if !settings.OTelEnabled {
		return nil, fmt.Errorf("OpenTelemetry integration is disabled")
	}
	if settings.OTelEndpoint == "" {
		return nil, fmt.Errorf("%s", MissingHint(IDOpenTelemetry))
	}
	backend := toolotel.Backend(strings.ToLower(strings.TrimSpace(settings.OTelBackend)))
	if backend == "" {
		backend = toolotel.BackendAuto
	}
	options = append([]toolotel.Option{toolotel.WithBackend(backend)}, options...)
	return toolotel.New(settings.OTelEndpoint, options...)
}
