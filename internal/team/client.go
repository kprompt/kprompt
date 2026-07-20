// Package team talks to the optional kprompt Team control plane (api.kprompt.ai).
// Without enrollment the CLI behaves as today’s OSS binary.
package team

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultAPIURL = "https://api.kprompt.ai"

// Client is a minimal HTTP client for device login / me / revoke.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Token      string
}

func NewClient(baseURL, token string) *Client {
	base := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if base == "" {
		base = DefaultAPIURL
	}
	return &Client{
		BaseURL: base,
		Token:   token,
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

type StartDeviceCodeResult struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

type PollDeviceTokenResult struct {
	Status    string  `json:"status"`
	APIToken  string  `json:"api_token,omitempty"`
	TokenHint string  `json:"token_hint,omitempty"`
	Org       *Org    `json:"org,omitempty"`
	Member    *Member `json:"member,omitempty"`
}

type Org struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Member struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Label string `json:"label"`
	Role  string `json:"role"`
}

type MeResponse struct {
	Org    Org    `json:"org"`
	Member Member `json:"member"`
	Auth   string `json:"auth"`
	Token  *struct {
		ID     string `json:"id"`
		Prefix string `json:"prefix"`
	} `json:"token,omitempty"`
}

type apiError struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *Client) StartDeviceCode(ctx context.Context) (StartDeviceCodeResult, error) {
	var out StartDeviceCodeResult
	err := c.doJSON(ctx, http.MethodPost, "/v1/device/code", nil, "", &out)
	return out, err
}

func (c *Client) PollDeviceToken(ctx context.Context, deviceCode string) (PollDeviceTokenResult, error) {
	var out PollDeviceTokenResult
	err := c.doJSON(ctx, http.MethodPost, "/v1/device/token", map[string]string{
		"device_code": deviceCode,
	}, "", &out)
	return out, err
}

func (c *Client) Me(ctx context.Context) (MeResponse, error) {
	var out MeResponse
	err := c.doJSON(ctx, http.MethodGet, "/v1/me", nil, c.Token, &out)
	return out, err
}

func (c *Client) Revoke(ctx context.Context) error {
	return c.doJSON(ctx, http.MethodPost, "/v1/tokens/revoke", map[string]any{}, c.Token, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, token string, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := c.HTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return err
	}
	// Pending poll returns 202 with a body we still decode.
	if res.StatusCode >= 400 {
		var ae apiError
		_ = json.Unmarshal(data, &ae)
		msg := strings.TrimSpace(ae.Error.Message)
		if msg == "" {
			msg = strings.TrimSpace(string(data))
		}
		if msg == "" {
			msg = res.Status
		}
		// Poll statuses expired/denied/consumed also come as 401 with JSON status.
		if out != nil && len(data) > 0 && json.Unmarshal(data, out) == nil {
			if pr, ok := out.(*PollDeviceTokenResult); ok && pr.Status != "" {
				return nil
			}
		}
		return fmt.Errorf("api %s: %s", res.Status, msg)
	}
	if out == nil || len(data) == 0 || string(data) == "null\n" {
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	return nil
}
