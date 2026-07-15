package llm

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

// OpenAI is an OpenAI-compatible chat completions client.
type OpenAI struct {
	apiKey  string
	baseURL string
	model   string
	client  *http.Client
}

func NewOpenAI(apiKey, baseURL, model string) *OpenAI {
	baseURL = strings.TrimRight(baseURL, "/")
	return &OpenAI{
		apiKey:  apiKey,
		baseURL: baseURL,
		model:   model,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

func (o *OpenAI) Name() string { return "openai" }

func (o *OpenAI) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = o.model
	}
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": req.System},
			{"role": "user", "content": req.User},
		},
	}
	raw, err := o.post(ctx, "/chat/completions", body)
	if err != nil {
		return CompletionResponse{}, err
	}
	text, err := extractOpenAIText(raw)
	if err != nil {
		return CompletionResponse{}, err
	}
	return CompletionResponse{Text: text}, nil
}

func (o *OpenAI) CompleteStructured(ctx context.Context, req CompletionRequest, schema json.RawMessage) (json.RawMessage, error) {
	model := req.Model
	if model == "" {
		model = o.model
	}
	sys := req.System
	if sys != "" {
		sys += "\n\n"
	}
	sys += "Respond with JSON only that matches the provided schema. No markdown."
	body := map[string]any{
		"model": model,
		"messages": []map[string]string{
			{"role": "system", "content": sys},
			{"role": "user", "content": req.User + "\n\nJSON schema:\n" + string(schema)},
		},
		"response_format": map[string]string{"type": "json_object"},
	}
	raw, err := o.post(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	text, err := extractOpenAIText(raw)
	if err != nil {
		return nil, err
	}
	text = stripCodeFence(text)
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("openai structured response is not valid JSON")
	}
	return json.RawMessage(text), nil
}

func (o *OpenAI) post(ctx context.Context, path string, body any) ([]byte, error) {
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, o.baseURL+path, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Authorization", "Bearer "+o.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := o.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("openai: %s: %s", resp.Status, truncate(string(data), 400))
	}
	return data, nil
}

func extractOpenAIText(raw []byte) (string, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("openai: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return "", fmt.Errorf("openai: empty choices")
	}
	return parsed.Choices[0].Message.Content, nil
}

func stripCodeFence(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		if i := strings.LastIndex(s, "```"); i >= 0 {
			s = s[:i]
		}
	}
	return strings.TrimSpace(s)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
