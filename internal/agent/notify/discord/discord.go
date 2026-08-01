// Package discord POSTs gated AgentAlerts to Discord webhooks.
//
// Credentials come from the environment (mounted from a Kubernetes Secret in-cluster):
//
//	KPROMPT_DISCORD_WEBHOOK_URL — Discord incoming webhook URL
//
// Retries transient failures; callers should log errors and keep the watch loop alive.
// Webhook URLs are credentials; never log them.
package discord

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

const EnvWebhookURL = "KPROMPT_DISCORD_WEBHOOK_URL"

const (
	defaultAttempts = 3
	defaultBackoff  = 500 * time.Millisecond
)

// Config holds Discord destination settings.
type Config struct {
	URL        string
	Attempts   int
	Backoff    time.Duration
	HTTPClient *http.Client
}

// ConfigFromEnv loads Discord settings from process env (Secret -> env in the agent pod).
func ConfigFromEnv() Config {
	return Config{URL: firstEnv(EnvWebhookURL, "DISCORD_WEBHOOK_URL")}
}

// Enabled reports whether a URL is configured.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.URL) != ""
}

// Client posts AgentAlert text to Discord.
type Client struct {
	cfg Config
}

// New returns a Discord client with retry defaults.
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

// Notify POSTs the AgentAlert as a Discord webhook message.
func (c *Client) Notify(ctx context.Context, alert incident.AgentAlert) error {
	if c == nil {
		return fmt.Errorf("discord: client is nil")
	}
	if !c.cfg.Enabled() {
		return fmt.Errorf("discord: %s is required", EnvWebhookURL)
	}
	if err := incident.ValidateAgentAlert(alert); err != nil {
		return err
	}
	raw, err := json.Marshal(payloadFor(alert))
	if err != nil {
		return err
	}
	return c.postWithRetry(ctx, raw)
}

func (c *Client) postWithRetry(ctx context.Context, raw []byte) error {
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
	return fmt.Errorf("discord: after %d attempts: %w", c.cfg.Attempts, last)
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
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 200 && res.StatusCode < 300 {
		return nil
	}
	msg := fmt.Sprintf("webhook HTTP %d: %s", res.StatusCode, truncate(string(respBody), 200))
	if res.StatusCode == 429 || res.StatusCode >= 500 {
		return &transientError{err: fmt.Errorf("%s", msg)}
	}
	return fmt.Errorf("discord: %s", msg)
}

type transientError struct{ err error }

func (e *transientError) Error() string { return e.err.Error() }
func (e *transientError) Unwrap() error { return e.err }

func retryable(err error) bool {
	_, ok := err.(*transientError)
	return ok
}

type webhookPayload struct {
	Content         string          `json:"content"`
	Username        string          `json:"username,omitempty"`
	AllowedMentions allowedMentions `json:"allowed_mentions"`
}

type allowedMentions struct {
	Parse []string `json:"parse"`
}

func payloadFor(a incident.AgentAlert) webhookPayload {
	return webhookPayload{
		Content:         truncate(FormatAlert(a), 2000),
		Username:        "kprompt Observe",
		AllowedMentions: allowedMentions{Parse: []string{}},
	}
}

// FormatAlert renders a compact Discord markdown message from AgentAlert.
func FormatAlert(a incident.AgentAlert) string {
	var b strings.Builder
	fmt.Fprintf(&b, "**%s** - `%s`\n", a.Namespace, a.Status)
	fmt.Fprintf(&b, "**Summary:** %s\n", a.Summary)
	fmt.Fprintf(&b, "**Severity:** %s - **Confidence:** %.0f%%\n", a.Severity, a.Confidence*100)
	if a.RootCause != "" {
		fmt.Fprintf(&b, "**Reason:** %s\n", a.RootCause)
	}
	if a.Recommendation != "" {
		fmt.Fprintf(&b, "**Recommendation:** %s\n", a.Recommendation)
	}
	if len(a.Affected) > 0 {
		parts := make([]string, 0, len(a.Affected))
		for _, r := range a.Affected {
			parts = append(parts, r.Kind+"/"+r.Name)
		}
		fmt.Fprintf(&b, "**Affected:** %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, "_incident %s - kprompt Observe_", a.IncidentID)
	return b.String()
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
