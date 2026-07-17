package grafana

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

var ErrResponseTooLarge = errors.New("Grafana response exceeds size limit")

// Client queries the Grafana HTTP API.
type Client struct {
	baseURL      *url.URL
	apiKey       string
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

// WithTimeout bounds each Grafana API request.
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

// New constructs a Grafana API client. apiKey may be empty for anonymous access.
func New(baseURL, apiKey string, options ...Option) (*Client, error) {
	raw := strings.TrimSpace(baseURL)
	if raw == "" {
		return nil, fmt.Errorf("Grafana base URL is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("Grafana base URL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("Grafana base URL scheme must be http or https")
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("Grafana base URL must include a host")
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	parsed.Path = strings.TrimRight(parsed.Path, "/")

	client := &Client{
		baseURL:      parsed,
		apiKey:       strings.TrimSpace(apiKey),
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

// ListDashboards searches dashboards with optional text and tag filters.
func (c *Client) ListDashboards(
	ctx context.Context,
	search SearchRequest,
) ([]DashboardSummary, error) {
	search.Query = strings.TrimSpace(search.Query)
	search.Tag = strings.TrimSpace(search.Tag)
	if search.Limit == 0 {
		search.Limit = 100
	}
	if search.Limit < 1 || search.Limit > 1000 {
		return nil, fmt.Errorf("Grafana dashboard limit must be between 1 and 1000")
	}
	query := url.Values{
		"type":  {"dash-db"},
		"limit": {strconv.Itoa(search.Limit)},
	}
	if search.Query != "" {
		query.Set("query", search.Query)
	}
	if search.Tag != "" {
		query.Set("tag", search.Tag)
	}

	var raw []searchDashboard
	if err := c.getJSON(ctx, "/api/search", query, &raw); err != nil {
		return nil, err
	}
	out := make([]DashboardSummary, 0, len(raw))
	for _, item := range raw {
		if item.Type != "" && item.Type != "dash-db" {
			continue
		}
		out = append(out, DashboardSummary{
			UID:         item.UID,
			Title:       item.Title,
			URL:         c.absoluteURL(item.URL),
			Tags:        append([]string(nil), item.Tags...),
			FolderUID:   item.FolderUID,
			FolderTitle: item.FolderTitle,
			Starred:     item.IsStarred,
		})
	}
	return out, nil
}

// GetDashboard fetches a dashboard and flattens nested row panels.
func (c *Client) GetDashboard(ctx context.Context, uid string) (Dashboard, error) {
	uid = strings.TrimSpace(uid)
	if uid == "" {
		return Dashboard{}, fmt.Errorf("Grafana dashboard UID is required")
	}
	if strings.Contains(uid, "/") {
		return Dashboard{}, fmt.Errorf("Grafana dashboard UID must not contain '/'")
	}

	var envelope dashboardEnvelope
	if err := c.getJSON(ctx, "/api/dashboards/uid/"+uid, nil, &envelope); err != nil {
		return Dashboard{}, err
	}
	return Dashboard{
		UID:    first(envelope.Dashboard.UID, uid),
		Title:  envelope.Dashboard.Title,
		URL:    c.absoluteURL(envelope.Meta.URL),
		Tags:   append([]string(nil), envelope.Dashboard.Tags...),
		Panels: flattenPanels(envelope.Dashboard.Panels),
	}, nil
}

func (c *Client) getJSON(
	ctx context.Context,
	apiPath string,
	query url.Values,
	target any,
) error {
	if c == nil || c.baseURL == nil {
		return fmt.Errorf("Grafana client is nil")
	}
	requestCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	endpoint := *c.baseURL
	endpoint.Path = path.Join(endpoint.Path, apiPath)
	if query != nil {
		endpoint.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(requestCtx, http.MethodGet, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("Grafana request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("Grafana request timed out after %s: %w", c.timeout, requestCtx.Err())
		}
		return fmt.Errorf("Grafana request: %w", err)
	}
	defer resp.Body.Close()

	body, err := readLimited(resp.Body, c.maxBodyBytes)
	if err != nil {
		return err
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return apiError(resp.StatusCode, body)
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("decode Grafana response (HTTP %d): %w", resp.StatusCode, err)
	}
	return nil
}

func readLimited(reader io.Reader, limit int64) ([]byte, error) {
	if limit <= 0 {
		limit = DefaultMaxBodyBytes
	}
	body, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, fmt.Errorf("read Grafana response: %w", err)
	}
	if int64(len(body)) > limit {
		return nil, ErrResponseTooLarge
	}
	return body, nil
}

func apiError(statusCode int, body []byte) error {
	var payload struct {
		Message string `json:"message"`
	}
	_ = json.Unmarshal(body, &payload)
	message := strings.TrimSpace(payload.Message)
	if message == "" {
		message = strings.TrimSpace(string(body))
	}
	if message == "" {
		message = http.StatusText(statusCode)
	}
	if len(message) > 512 {
		message = message[:512] + "…"
	}
	return fmt.Errorf("Grafana API error (HTTP %d): %s", statusCode, message)
}

func flattenPanels(raw []rawPanel) []Panel {
	var out []Panel
	var appendPanels func([]rawPanel)
	appendPanels = func(panels []rawPanel) {
		for _, panel := range panels {
			if panel.Type == "row" && len(panel.Panels) > 0 {
				appendPanels(panel.Panels)
				continue
			}
			out = append(out, normalizePanel(panel))
			if len(panel.Panels) > 0 {
				appendPanels(panel.Panels)
			}
		}
	}
	appendPanels(raw)
	return out
}

func normalizePanel(raw rawPanel) Panel {
	targets := make([]Target, 0, len(raw.Targets))
	for _, target := range raw.Targets {
		targets = append(targets, Target{
			RefID:      target.RefID,
			Datasource: datasourceLabel(target.Datasource),
			Expression: first(target.Expr, target.Query),
			Hidden:     target.Hide,
		})
	}
	return Panel{
		ID:         raw.ID,
		Title:      raw.Title,
		Type:       raw.Type,
		Datasource: normalizeDatasource(raw.Datasource),
		Grid:       raw.GridPos,
		Targets:    targets,
	}
}

func normalizeDatasource(raw json.RawMessage) Datasource {
	if len(raw) == 0 || string(raw) == "null" {
		return Datasource{}
	}
	var name string
	if json.Unmarshal(raw, &name) == nil {
		return Datasource{Name: name}
	}
	var object struct {
		UID  string `json:"uid"`
		Type string `json:"type"`
		Name string `json:"name"`
	}
	if json.Unmarshal(raw, &object) != nil {
		return Datasource{}
	}
	return Datasource{UID: object.UID, Type: object.Type, Name: object.Name}
}

func datasourceLabel(raw json.RawMessage) string {
	source := normalizeDatasource(raw)
	return first(source.UID, source.Name, source.Type)
}

func (c *Client) absoluteURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || c == nil || c.baseURL == nil {
		return raw
	}
	reference, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	base := *c.baseURL
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	return base.ResolveReference(reference).String()
}

func first(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
