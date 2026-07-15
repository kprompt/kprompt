package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Gemini is a Google AI Studio / Gemini API client.
type Gemini struct {
	apiKey string
	model  string
	client *http.Client
}

func NewGemini(apiKey, model string) *Gemini {
	return &Gemini{
		apiKey: apiKey,
		model:  model,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

func (g *Gemini) Name() string { return "gemini" }

func (g *Gemini) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	model := req.Model
	if model == "" {
		model = g.model
	}
	body := map[string]any{
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{
					{"text": req.User},
				},
			},
		},
	}
	if strings.TrimSpace(req.System) != "" {
		body["systemInstruction"] = map[string]any{
			"parts": []map[string]string{{"text": req.System}},
		}
	}
	raw, err := g.post(ctx, model, body)
	if err != nil {
		return CompletionResponse{}, err
	}
	text, err := extractGeminiText(raw)
	if err != nil {
		return CompletionResponse{}, err
	}
	return CompletionResponse{Text: text}, nil
}

func (g *Gemini) CompleteStructured(ctx context.Context, req CompletionRequest, schema json.RawMessage) (json.RawMessage, error) {
	sys := req.System
	if sys != "" {
		sys += "\n\n"
	}
	sys += "Respond with JSON only that matches the provided schema. No markdown."
	user := req.User + "\n\nJSON schema:\n" + string(schema)
	model := req.Model
	if model == "" {
		model = g.model
	}
	body := map[string]any{
		"systemInstruction": map[string]any{
			"parts": []map[string]string{{"text": sys}},
		},
		"contents": []map[string]any{
			{
				"role": "user",
				"parts": []map[string]string{{"text": user}},
			},
		},
		"generationConfig": map[string]any{
			"responseMimeType": "application/json",
		},
	}
	raw, err := g.post(ctx, model, body)
	if err != nil {
		return nil, err
	}
	text, err := extractGeminiText(raw)
	if err != nil {
		return nil, err
	}
	text = stripCodeFence(text)
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("gemini structured response is not valid JSON")
	}
	return json.RawMessage(text), nil
}

func (g *Gemini) post(ctx context.Context, model string, body any) ([]byte, error) {
	model = strings.TrimPrefix(model, "models/")
	endpoint := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/%s:generateContent?key=%s",
		url.PathEscape(model),
		url.QueryEscape(g.apiKey),
	)
	b, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := g.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gemini: %s: %s", resp.Status, truncate(string(data), 400))
	}
	return data, nil
}

func extractGeminiText(raw []byte) (string, error) {
	var parsed struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "", err
	}
	if parsed.Error != nil {
		return "", fmt.Errorf("gemini: %s", parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return "", fmt.Errorf("gemini: empty candidates")
	}
	var b strings.Builder
	for _, p := range parsed.Candidates[0].Content.Parts {
		b.WriteString(p.Text)
	}
	return b.String(), nil
}
