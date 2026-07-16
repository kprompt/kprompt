package tools

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

var errEmptyURL = errors.New("empty url")

func detectPrometheus(ctx context.Context, settings Settings) Result {
	r := Result{
		ID:           IDPrometheus,
		Name:         "Prometheus",
		Capabilities: []Capability{CapQuery},
	}
	if !settings.PrometheusEnabled {
		r.Status = StatusDisabled
		r.Detail = "disabled in config or KPROMPT_PROMETHEUS_ENABLED=0"
		r.Hint = MissingHint(IDPrometheus)
		return r
	}
	if settings.PrometheusURL == "" {
		r.Status = StatusUnavailable
		r.Detail = "KPROMPT_PROMETHEUS_URL not set"
		r.Hint = MissingHint(IDPrometheus)
		return r
	}
	if err := probeHTTPPath(ctx, settings.PrometheusURL, "/-/healthy"); err != nil {
		if err2 := probeHTTPPath(ctx, settings.PrometheusURL, "/api/v1/status/config"); err2 != nil {
			r.Status = StatusConfigured
			r.Detail = fmt.Sprintf("%s (probe failed: %v)", settings.PrometheusURL, err)
			return r
		}
	}
	r.Status = StatusAvailable
	r.Detail = settings.PrometheusURL
	return r
}

func detectGrafana(ctx context.Context, settings Settings) Result {
	r := Result{
		ID:           IDGrafana,
		Name:         "Grafana",
		Capabilities: []Capability{CapQuery},
	}
	if !settings.GrafanaEnabled {
		r.Status = StatusDisabled
		r.Detail = "disabled in config or KPROMPT_GRAFANA_ENABLED=0"
		r.Hint = MissingHint(IDGrafana)
		return r
	}
	if settings.GrafanaURL == "" {
		r.Status = StatusUnavailable
		r.Detail = "KPROMPT_GRAFANA_URL not set"
		r.Hint = MissingHint(IDGrafana)
		return r
	}
	if err := probeHTTPPath(ctx, settings.GrafanaURL, "/api/health"); err != nil {
		r.Status = StatusConfigured
		r.Detail = fmt.Sprintf("%s (probe failed: %v)", settings.GrafanaURL, err)
		return r
	}
	r.Status = StatusAvailable
	r.Detail = settings.GrafanaURL
	return r
}

func detectOTel(settings Settings) Result {
	r := Result{
		ID:           IDOpenTelemetry,
		Name:         "OpenTelemetry",
		Capabilities: []Capability{CapQuery},
	}
	if !settings.OTelEnabled {
		r.Status = StatusDisabled
		r.Detail = "disabled in config or KPROMPT_OTEL_ENABLED=0"
		r.Hint = MissingHint(IDOpenTelemetry)
		return r
	}
	if settings.OTelEndpoint == "" {
		r.Status = StatusUnavailable
		r.Detail = "KPROMPT_OTEL_ENDPOINT not set"
		r.Hint = MissingHint(IDOpenTelemetry)
		return r
	}
	r.Status = StatusConfigured
	r.Detail = fmt.Sprintf("%s (backend: %s)", settings.OTelEndpoint, settings.OTelBackend)
	return r
}

func newProbeRequest(ctx context.Context, url string) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return req, nil
}

func doProbe(req *http.Request) error {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= 500 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return nil
}
