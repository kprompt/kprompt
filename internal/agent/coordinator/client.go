package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OutcomeReader reads the Coordinator cross-ns outcome summary (RT-022).
// Consumed by namespace agents as evidence-not-proof bias only (AG-034).
type OutcomeReader interface {
	Outcomes(ctx context.Context) (OutcomeSummary, error)
}

// HTTPClient is a read-only client for the Coordinator HTTP API (RT-022).
type HTTPClient struct {
	// BaseURL is the Coordinator root (no path), e.g. http://kprompt-coordinator:9090.
	BaseURL    string
	HTTPClient *http.Client
}

// NormalizeBaseURL strips a trailing handoff path so a handoff URL can be reused
// as the outcome-read base (RT-022). "http://x:9090/v1/handoff" → "http://x:9090".
func NormalizeBaseURL(raw string) string {
	u := strings.TrimRight(strings.TrimSpace(raw), "/")
	for _, suffix := range []string{"/v1/handoff", "/v1/outcome", "/v1/outcomes"} {
		if strings.HasSuffix(u, suffix) {
			u = strings.TrimSuffix(u, suffix)
			break
		}
	}
	return strings.TrimRight(u, "/")
}

// Outcomes fetches GET /v1/outcomes and decodes an OutcomeSummary (RT-022).
func (c *HTTPClient) Outcomes(ctx context.Context) (OutcomeSummary, error) {
	if c == nil {
		return OutcomeSummary{}, fmt.Errorf("coordinator client: nil")
	}
	base := NormalizeBaseURL(c.BaseURL)
	if base == "" {
		return OutcomeSummary{}, fmt.Errorf("coordinator client: BaseURL required")
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/outcomes", nil)
	if err != nil {
		return OutcomeSummary{}, err
	}
	res, err := hc.Do(req)
	if err != nil {
		return OutcomeSummary{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, maxBody))
	if err != nil {
		return OutcomeSummary{}, fmt.Errorf("coordinator client: read: %w", err)
	}
	if res.StatusCode >= 300 {
		return OutcomeSummary{}, fmt.Errorf("coordinator client: HTTP %d", res.StatusCode)
	}
	var sum OutcomeSummary
	if err := json.Unmarshal(body, &sum); err != nil {
		return OutcomeSummary{}, fmt.Errorf("coordinator client: decode: %w", err)
	}
	return sum, nil
}
