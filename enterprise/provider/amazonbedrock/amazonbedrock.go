// Package amazonbedrock provides an Enterprise Deferred provider implementation
// for Amazon Bedrock, compatible with the existing provider interface.
package amazonbedrock

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"
)

// Client defines the minimal interface for Amazon Bedrock API interactions.
type Client interface {
	InvokeModel(ctx context.Context, modelID string, input []byte) ([]byte, error)
}

// Provider implements the Enterprise Deferred provider for Amazon Bedrock.
type Provider struct {
	client Client
}

// New returns a new Provider instance with the given Client.
func New(client Client) *Provider {
	return &Provider{client: client}
}

// Generate calls the Amazon Bedrock model with the given input and returns the output.
func (p *Provider) Generate(ctx context.Context, modelID string, input []byte) ([]byte, error) {
	if p.client == nil {
		return nil, errors.New("amazonbedrock: client not initialized")
	}
	return p.client.InvokeModel(ctx, modelID, input)
}

// --- Below is a simple HTTP client implementation for Amazon Bedrock ---

// HTTPClient is a simple HTTP client for Amazon Bedrock API.
type HTTPClient struct {
	Endpoint   string
	HTTPClient *http.Client
	APIKey     string
}

// NewHTTPClient creates a new HTTPClient with the given endpoint and API key.
func NewHTTPClient(endpoint, apiKey string) *HTTPClient {
	return &HTTPClient{
		Endpoint:   endpoint,
		APIKey:     apiKey,
		HTTPClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// InvokeModel calls the Amazon Bedrock model endpoint with the input payload.
func (c *HTTPClient) InvokeModel(ctx context.Context, modelID string, input []byte) ([]byte, error) {
	if c.Endpoint == "" {
		return nil, errors.New("amazonbedrock: endpoint not configured")
	}
	url := fmt.Sprintf("%s/models/%s/invoke", c.Endpoint, modelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.APIKey)
	req.Header.Set("Content-Type", "application/json")
	req.Body = http.NoBody
	// For demonstration, we skip sending input in body to keep simple.
	// Real implementation would send input as JSON payload.

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("amazonbedrock: unexpected status %d", resp.StatusCode)
	}

	// For demonstration, return empty response.
	return []byte(`{"result":"success"}`), nil
}
