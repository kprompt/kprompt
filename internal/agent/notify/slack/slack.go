// Package slack posts gated AgentAlerts to Slack with thread updates (AG-009).
//
// Credentials come from the environment (mounted from a Kubernetes Secret in-cluster):
//
//	KPROMPT_SLACK_BOT_TOKEN + KPROMPT_SLACK_CHANNEL  — chat.postMessage (threaded)
//	KPROMPT_SLACK_WEBHOOK_URL                        — incoming webhook (no reliable thread ts)
//
// Prefer bot token for Observe thread updates. Webhook mode posts standalone messages.
package slack

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
	EnvWebhookURL = "KPROMPT_SLACK_WEBHOOK_URL"
	EnvBotToken   = "KPROMPT_SLACK_BOT_TOKEN"
	EnvChannel    = "KPROMPT_SLACK_CHANNEL"

	apiPostMessage = "https://slack.com/api/chat.postMessage"
)

// Config holds Slack credentials (never log the token).
type Config struct {
	WebhookURL string
	BotToken   string
	Channel    string
	// APIURL overrides chat.postMessage (tests).
	APIURL string
	// HTTPClient defaults to a 15s client.
	HTTPClient *http.Client
}

// ConfigFromEnv loads Slack settings from process env (Secret → env in the agent pod).
func ConfigFromEnv() Config {
	return Config{
		WebhookURL: firstEnv(EnvWebhookURL, "SLACK_WEBHOOK_URL"),
		BotToken:   firstEnv(EnvBotToken, "SLACK_BOT_TOKEN"),
		Channel:    firstEnv(EnvChannel, "SLACK_CHANNEL"),
	}
}

// Enabled is true when either bot or webhook credentials are present.
func (c Config) Enabled() bool {
	return strings.TrimSpace(c.WebhookURL) != "" ||
		(strings.TrimSpace(c.BotToken) != "" && strings.TrimSpace(c.Channel) != "")
}

// Threaded is true when bot token mode can maintain threads.
func (c Config) Threaded() bool {
	return strings.TrimSpace(c.BotToken) != "" && strings.TrimSpace(c.Channel) != ""
}

// Client posts AgentAlerts.
type Client struct {
	cfg Config
}

// New returns a Slack client.
func New(cfg Config) *Client {
	if cfg.HTTPClient == nil {
		cfg.HTTPClient = &http.Client{Timeout: 15 * time.Second}
	}
	if cfg.APIURL == "" {
		cfg.APIURL = apiPostMessage
	}
	return &Client{cfg: cfg}
}

// PostResult captures Slack message identity for thread follow-ups.
type PostResult struct {
	ThreadTS string `json:"threadTs,omitempty"`
	Channel  string `json:"channel,omitempty"`
	Mode     string `json:"mode"` // bot | webhook
}

// Notify posts an alert. When threadTS is set (bot mode), replies in that thread.
// Returns the root thread timestamp to store on the Incident.
func (c *Client) Notify(ctx context.Context, alert incident.AgentAlert, threadTS string) (PostResult, error) {
	if c == nil {
		return PostResult{}, fmt.Errorf("slack: client is nil")
	}
	if err := incident.ValidateAgentAlert(alert); err != nil {
		return PostResult{}, err
	}
	text := FormatAlert(alert)

	if c.cfg.Threaded() {
		return c.postBot(ctx, text, threadTS)
	}
	if strings.TrimSpace(c.cfg.WebhookURL) != "" {
		if err := c.postWebhook(ctx, text, threadTS); err != nil {
			return PostResult{}, err
		}
		return PostResult{ThreadTS: threadTS, Mode: "webhook"}, nil
	}
	return PostResult{}, fmt.Errorf("slack: set %s or %s+%s (from Secret/env)", EnvBotToken, EnvChannel, EnvWebhookURL)
}

// PostText posts a plain-text reply (AG-019 ask). Requires bot token mode.
func (c *Client) PostText(ctx context.Context, text, threadTS string) (PostResult, error) {
	if c == nil {
		return PostResult{}, fmt.Errorf("slack: client is nil")
	}
	if !c.cfg.Threaded() {
		return PostResult{}, fmt.Errorf("slack ask requires %s + %s (bot token mode)", EnvBotToken, EnvChannel)
	}
	return c.postBot(ctx, text, threadTS)
}

// Channel returns the configured Slack channel id/name.
func (c *Client) Channel() string {
	if c == nil {
		return ""
	}
	return c.cfg.Channel
}

// Threaded reports whether bot-token mode can maintain threads.
func (c *Client) Threaded() bool {
	if c == nil {
		return false
	}
	return c.cfg.Threaded()
}

func (c *Client) postBot(ctx context.Context, text, threadTS string) (PostResult, error) {
	body := map[string]any{
		"channel": c.cfg.Channel,
		"text":    text,
	}
	if threadTS != "" {
		body["thread_ts"] = threadTS
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return PostResult{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.APIURL, bytes.NewReader(raw))
	if err != nil {
		return PostResult{}, err
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("Authorization", "Bearer "+c.cfg.BotToken)

	res, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return PostResult{}, err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return PostResult{}, fmt.Errorf("slack: chat.postMessage HTTP %d: %s", res.StatusCode, truncate(string(respBody), 200))
	}
	var parsed struct {
		OK      bool   `json:"ok"`
		Error   string `json:"error"`
		TS      string `json:"ts"`
		Channel string `json:"channel"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return PostResult{}, fmt.Errorf("slack: decode response: %w", err)
	}
	if !parsed.OK {
		return PostResult{}, fmt.Errorf("slack: api error: %s", firstNonEmpty(parsed.Error, "unknown"))
	}
	root := threadTS
	if root == "" {
		root = parsed.TS
	}
	return PostResult{ThreadTS: root, Channel: parsed.Channel, Mode: "bot"}, nil
}

func (c *Client) postWebhook(ctx context.Context, text, threadTS string) error {
	body := map[string]any{"text": text}
	if threadTS != "" {
		body["thread_ts"] = threadTS
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.WebhookURL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := c.cfg.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("slack: webhook HTTP %d: %s", res.StatusCode, truncate(string(respBody), 200))
	}
	return nil
}

// FormatAlert renders a compact Slack mrkdwn message from AgentAlert.
func FormatAlert(a incident.AgentAlert) string {
	icon := "⚠️"
	switch strings.ToLower(a.Severity) {
	case incident.SeverityCritical, incident.SeverityHigh:
		icon = "🚨"
	case incident.SeverityInfo, incident.SeverityLow:
		icon = "ℹ️"
	}
	if a.Status == incident.AlertRecovered {
		icon = "✅"
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s *%s* · `%s`\n", icon, a.Namespace, a.Status)
	fmt.Fprintf(&b, "*Summary:* %s\n", a.Summary)
	fmt.Fprintf(&b, "*Severity:* %s · *Confidence:* %.0f%%\n", a.Severity, a.Confidence*100)
	if a.RootCause != "" {
		fmt.Fprintf(&b, "*Reason:* %s\n", a.RootCause)
	}
	if a.Recommendation != "" {
		fmt.Fprintf(&b, "*Recommendation:* %s\n", a.Recommendation)
	}
	if len(a.Affected) > 0 {
		parts := make([]string, 0, len(a.Affected))
		for _, r := range a.Affected {
			parts = append(parts, r.Kind+"/"+r.Name)
		}
		fmt.Fprintf(&b, "*Affected:* %s\n", strings.Join(parts, ", "))
	}
	fmt.Fprintf(&b, "_incident %s · kprompt Observe_", a.IncidentID)
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
