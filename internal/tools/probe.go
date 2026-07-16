package tools

import (
	"context"
	"strings"
)

func probeHTTPPath(ctx context.Context, baseURL, path string) error {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return errEmptyURL
	}
	req, err := newProbeRequest(ctx, baseURL+path)
	if err != nil {
		return err
	}
	return doProbe(req)
}
