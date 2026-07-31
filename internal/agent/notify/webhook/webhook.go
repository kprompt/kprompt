// Package webhook POSTs gated AgentAlert JSON to a generic HTTPS endpoint (AG-010).
//
// This is the adapter hook for Teams / Discord / PagerDuty / Opsgenie / Jira
// without coupling those vendors into the Observe core.
//
//	KPROMPT_WEBHOOK_URL — destination (from Secret/env)
//
// Retries transient failures; callers should log errors and keep the watch loop alive.
package webhook

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/incident"
)

const (
	EnvURL = "KPROMPT_WEBHOOK_URL"

	defaultAttempts = 3
	defaultBackoff  = 500 * time.Millisecond
)

// Config holds webhook destination settings.
type Config struct {
	URL        string
	Attempts   int
	Backoff    time.Duration
	HTTPClient *http.Client
}

// ConfigFromEnv loads KPROMPT_WEBHOOK_URL.
func ConfigFromEnv() Config {
	return Config{URL: strings.TrimSpace(os.Getenv(EnvURL))}
}

// Enabled reports whether a URL is configured.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.URL) != ""
}

// Client posts AgentAlert JSON.
type Client struct {
	cfg Config
}

// New returns a webhook client with retry defaults.
func New(cfg Config) *Client {
	if cfg.Attempts <= 0 {
		cfg.Attempts = defaultAttempts
	}
	if cfg.Backoff <= 0 {
		cfg.Backoff = defaultBackoff
	}
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	return &Client{cfg: cfg}
}

// Notify POSTs the AgentAlert as application/json.
func (c *Client) Notify(ctx context.Context, alert incident.AgentAlert) error {
	if c == nil {
		return fmt.Errorf("webhook: client is nil")
	}
	if !c.cfg.Enabled() {
		return fmt.Errorf("webhook: %s is required", EnvURL)
	}
	if err := incident.ValidateAgentAlert(alert); err != nil {
		return err
	}
	return c.NotifyJSON(ctx, alert)
}

// NotifyJSON POSTs an arbitrary JSON payload (AG-053 CoordinatorReply follow-up).
func (c *Client) NotifyJSON(ctx context.Context, payload any) error {
	if c == nil {
		return fmt.Errorf("webhook: client is nil")
	}
	if !c.cfg.Enabled() {
		return fmt.Errorf("webhook: %s is required", EnvURL)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	var last error
	backoff := c.cfg.Backoff
	for attempt := 1; attempt <= c.cfg.Attempts; attempt++ {
		last = c.postOnce(ctx, raw)
		if last == nil {
			return nil
		}
		if attempt == c.cfg.Attempts || ctx.Err() != nil {
			break
		}
		if !retryable(last) {
			return last
		}
		t := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
		backoff *= 2
	}
	return fmt.Errorf("webhook: after %d attempts: %w", c.cfg.Attempts, last)
}

func (c *Client) postOnce(ctx context.Context, raw []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "kprompt-agent/observe")
	res, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return &transientError{err: err}
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	msg := fmt.Sprintf("HTTP %d: %s", res.StatusCode, truncate(string(body), 200))
	if res.StatusCode == 429 || res.StatusCode >= 500 {
		return &transientError{err: fmt.Errorf("%s", msg)}
	}
	return fmt.Errorf("webhook: %s", msg)
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func retryable(err error) bool {
	_, ok := err.(*transientError)
	return ok
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
