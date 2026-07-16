package prometheus

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultTimeout      = 10 * time.Second
	DefaultMaxBodyBytes = int64(4 << 20)
)

var ErrResponseTooLarge = errors.New("prometheus response exceeds size limit")

// Client queries the Prometheus v1 HTTP API.
type Client struct {
	baseURL      *url.URL
	httpClient   *http.Client
	timeout      time.Duration
	maxBodyBytes int64
}

// Option configures a Client.
type Option func(*Client)

// WithHTTPClient replaces the default HTTP client.
func WithHTTPClient(client *http.Client) Option {
	return func(c *Client) {
		if client != nil {
			c.httpClient = client
		}
	}
}

// WithTimeout bounds each Prometheus API request.
func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) {
		if timeout > 0 {
			c.timeout = timeout
		}
	}
}

// WithMaxBodyBytes caps response bodies to protect CLI memory use.
func WithMaxBodyBytes(limit int64) Option {
	return func(c *Client) {
		if limit > 0 {
			c.maxBodyBytes = limit
		}
	}
}

// New constructs a Prometheus query client.
func New(baseURL string, options ...Option) (*Client, error) {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return nil, fmt.Errorf("prometheus base URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("prometheus base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("prometheus base URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("prometheus base URL must include a host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	client := &Client{
		baseURL:      parsed,
		httpClient:   &http.Client{},
		timeout:      DefaultTimeout,
		maxBodyBytes: DefaultMaxBodyBytes,
	}
	for _, option := range options {
		if option != nil {
			option(client)
		}
	}
	return client, nil
}

// Query executes an instant PromQL query. A zero time uses Prometheus server time.
func (c *Client) Query(ctx context.Context, promQL string, at time.Time) (Result, error) {
	values := url.Values{"query": {strings.TrimSpace(promQL)}}
	if values.Get("query") == "" {
		return Result{}, fmt.Errorf("promQL query is required")
	}
	if !at.IsZero() {
		values.Set("time", formatTime(at))
	}
	return c.query(ctx, "/api/v1/query", values)
}

// QueryRange executes a range PromQL query.
func (c *Client) QueryRange(
	ctx context.Context,
	promQL string,
	start time.Time,
	end time.Time,
	step time.Duration,
) (Result, error) {
	promQL = strings.TrimSpace(promQL)
	if promQL == "" {
		return Result{}, fmt.Errorf("promQL query is required")
	}
	if start.IsZero() || end.IsZero() {
		return Result{}, fmt.Errorf("query range start and end are required")
	}
	if !end.After(start) {
		return Result{}, fmt.Errorf("query range end must be after start")
	}
	if step <= 0 {
		return Result{}, fmt.Errorf("query range step must be positive")
	}
	values := url.Values{
		"query": {promQL},
		"start": {formatTime(start)},
		"end":   {formatTime(end)},
		"step":  {formatDuration(step)},
	}
	return c.query(ctx, "/api/v1/query_range", values)
}

func (c *Client) query(ctx context.Context, apiPath string, values url.Values) (Result, error) {
	if c == nil || c.baseURL == nil {
		return Result{}, fmt.Errorf("prometheus client is nil")
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, apiPath)
	endpoint.RawQuery = values.Encode()

	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return Result{}, fmt.Errorf("prometheus request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return Result{}, fmt.Errorf("prometheus query timed out after %s: %w", c.timeout, requestCtx.Err())
		}
		return Result{}, fmt.Errorf("prometheus query: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimited(resp.Body, c.maxBodyBytes)
	if err != nil {
		return Result{}, err
	}
	var envelope apiEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return Result{}, fmt.Errorf("decode Prometheus response (HTTP %d): %w", resp.StatusCode, err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Result{}, apiError(resp.StatusCode, envelope)
	}
	if envelope.Status != "success" {
		return Result{}, apiError(resp.StatusCode, envelope)
	}
	result, err := decodeResult(envelope.Data)
	if err != nil {
		return Result{}, err
	}
	result.Warnings = append([]string(nil), envelope.Warnings...)
	return result, nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = DefaultMaxBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read Prometheus response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, ErrResponseTooLarge
	}
	return body, nil
}

func apiError(statusCode int, envelope apiEnvelope) error {
	message := strings.TrimSpace(envelope.Error)
	if message == "" {
		message = http.StatusText(statusCode)
	}
	if envelope.ErrorType != "" {
		return fmt.Errorf("Prometheus API %s error (HTTP %d): %s", envelope.ErrorType, statusCode, message)
	}
	return fmt.Errorf("Prometheus API error (HTTP %d): %s", statusCode, message)
}

func formatTime(value time.Time) string {
	return strconv.FormatFloat(float64(value.UnixNano())/float64(time.Second), 'f', -1, 64)
}

func formatDuration(value time.Duration) string {
	return strconv.FormatFloat(value.Seconds(), 'f', -1, 64)
}
