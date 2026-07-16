package otel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

const (
	DefaultTimeout      = 10 * time.Second
	DefaultMaxBodyBytes = int64(8 << 20)
)

var ErrResponseTooLarge = errors.New("trace response exceeds size limit")

// Client queries Jaeger or Tempo HTTP APIs.
type Client struct {
	endpoint     *url.URL
	backend      Backend
	httpClient   *http.Client
	timeout      time.Duration
	maxBodyBytes int64
}

// Option configures a Client.
type Option func(*Client)

// WithBackend selects auto, jaeger, or tempo.
func WithBackend(backend Backend) Option {
	return func(c *Client) {
		c.backend = backend
	}
}

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithTimeout bounds each backend request.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// WithMaxBodyBytes caps trace response bodies.
func WithMaxBodyBytes(limit int64) Option {
	return func(c *Client) {
		if limit > 0 {
			c.maxBodyBytes = limit
		}
	}
}

// New constructs a Jaeger/Tempo query client.
func New(endpoint string, options ...Option) (*Client, error) {
	raw := strings.TrimSpace(endpoint)
	if raw == "" {
		return nil, fmt.Errorf("trace backend endpoint is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("trace backend endpoint: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("trace backend endpoint scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("trace backend endpoint must include a host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	client := &Client{
		endpoint:     parsed,
		backend:      BackendAuto,
		httpClient:   &http.Client{},
		timeout:      DefaultTimeout,
		maxBodyBytes: DefaultMaxBodyBytes,
	}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}
	switch client.backend {
	case BackendAuto, BackendJaeger, BackendTempo:
	default:
		return nil, fmt.Errorf("unsupported trace backend %q", client.backend)
	}
	return client, nil
}

// SearchTraces fetches recent traces by service and optional operation.
func (c *Client) SearchTraces(ctx context.Context, req SearchRequest) ([]Trace, error) {
	req, err := normalizeSearchRequest(req)
	if err != nil {
		return nil, err
	}
	switch c.backend {
	case BackendJaeger:
		return c.searchJaeger(ctx, req)
	case BackendTempo:
		return c.searchTempo(ctx, req)
	case BackendAuto:
		traces, err := c.searchJaeger(ctx, req)
		if err == nil {
			return traces, nil
		}
		var statusErr HTTPStatusError
		if !errors.As(err, &statusErr) ||
			(statusErr.StatusCode != http.StatusNotFound &&
				statusErr.StatusCode != http.StatusMethodNotAllowed) {
			return nil, err
		}
		return c.searchTempo(ctx, req)
	default:
		return nil, fmt.Errorf("unsupported trace backend %q", c.backend)
	}
}

// GetTrace fetches and normalizes one trace by ID.
func (c *Client) GetTrace(ctx context.Context, traceID string) (Trace, error) {
	traceID = strings.TrimSpace(traceID)
	if traceID == "" {
		return Trace{}, fmt.Errorf("trace ID is required")
	}
	body, err := c.get(ctx, "/api/traces/"+url.PathEscape(traceID), nil)
	if err != nil {
		return Trace{}, err
	}
	switch c.backend {
	case BackendJaeger:
		return decodeJaegerTrace(body)
	case BackendTempo:
		return decodeTempoTrace(traceID, body)
	case BackendAuto:
		if trace, decodeErr := decodeJaegerTrace(body); decodeErr == nil {
			return trace, nil
		}
		return decodeTempoTrace(traceID, body)
	default:
		return Trace{}, fmt.Errorf("unsupported trace backend %q", c.backend)
	}
}

// HTTPStatusError reports a non-success backend response.
type HTTPStatusError struct {
	StatusCode int
	Body       string
}

func (e HTTPStatusError) Error() string {
	if e.Body == "" {
		return fmt.Sprintf("trace backend HTTP %d", e.StatusCode)
	}
	return fmt.Sprintf("trace backend HTTP %d: %s", e.StatusCode, e.Body)
}

func (c *Client) get(ctx context.Context, apiPath string, query url.Values) ([]byte, error) {
	if c == nil || c.endpoint == nil {
		return nil, fmt.Errorf("trace client is nil")
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := *c.endpoint
	endpoint.Path = path.Join(endpoint.Path, apiPath)
	endpoint.RawQuery = query.Encode()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("trace backend request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return nil, fmt.Errorf("trace backend timed out after %s: %w", c.timeout, requestCtx.Err())
		}
		return nil, fmt.Errorf("trace backend request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimited(resp.Body, c.maxBodyBytes)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, HTTPStatusError{
			StatusCode: resp.StatusCode,
			Body:       backendErrorBody(body),
		}
	}
	return body, nil
}

func normalizeSearchRequest(req SearchRequest) (SearchRequest, error) {
	req.Service = strings.TrimSpace(req.Service)
	req.Operation = strings.TrimSpace(req.Operation)
	if req.Service == "" {
		return SearchRequest{}, fmt.Errorf("trace service is required")
	}
	if req.End.IsZero() {
		req.End = time.Now()
	}
	if req.Start.IsZero() {
		req.Start = req.End.Add(-time.Hour)
	}
	if !req.End.After(req.Start) {
		return SearchRequest{}, fmt.Errorf("trace search end must be after start")
	}
	if req.End.Sub(req.Start) > 24*time.Hour {
		return SearchRequest{}, fmt.Errorf("trace search range must not exceed 24h")
	}
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Limit < 1 || req.Limit > 100 {
		return SearchRequest{}, fmt.Errorf("trace search limit must be between 1 and 100")
	}
	return req, nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = DefaultMaxBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read trace backend response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, ErrResponseTooLarge
	}
	return body, nil
}

func backendErrorBody(body []byte) string {
	var payload map[string]any
	if json.Unmarshal(body, &payload) == nil {
		for _, key := range []string{"error", "message"} {
			if value, ok := payload[key].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	const max = 512
	text := strings.TrimSpace(string(body))
	if len(text) > max {
		text = text[:max] + "…"
	}
	return text
}
