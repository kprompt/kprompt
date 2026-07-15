package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Anthropic is an Anthropic Messages API client.
type Anthropic struct {
	apiKey string
	model  string
	client *http.Client
}

func NewAnthropic(apiKey, model string) *Anthropic {
	return &Anthropic{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (a *Anthropic) Name() string { return "anthropic" }

func (a *Anthropic) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = a.model
	}
	body := map[string]any{
		"model":      model,
		"max_tokens": 2048,
		"system":     req.System,
		"messages": []map[string]string{
			{"role": "user", "content": req.User},
		},
	}
	raw, err := a.post(ctx, body)
	if err != nil {
		return CompletionResponse{}, err
	}
	text, err := extractAnthropicText(raw)
	if err != nil {
		return CompletionResponse{}, err
	}
	return CompletionResponse{Text: text}, nil
}

func (a *Anthropic) CompleteStructured(ctx context.Context, req CompletionRequest, schema json.RawMessage) (json.RawMessage, error) {
	sys := req.System
	if sys != "" {
		sys += "\n\n"
	}
	sys += "Respond with JSON only that matches the provided schema. No markdown."
	user := req.User + "\n\nJSON schema:\n" + string(schema)
	resp, err := a.Complete(ctx, CompletionRequest{System: sys, User: user, Model: req.Model})
	if err != nil {
		return nil, err
	}
	text := stripCodeFence(resp.Text)
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("anthropic structured response is not valid JSON")
	}
	return json.RawMessage(text), nil
}

func (a *Anthropic) post(ctx context.Context, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("x-api-key", a.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := a.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("anthropic: %s: %s", resp.Status, truncate(string(data), 400))
	}
	return data, nil
}

func extractAnthropicText(raw []byte) (string, error) {
	var parsed struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("anthropic: %s", parsed.Error.Message)
	}
	for _, c := range parsed.Content {
		if c.Type == "text" && c.Text != "" {
			return c.Text, nil
		}
	}
	return "", fmt.Errorf("anthropic: empty content")
}
