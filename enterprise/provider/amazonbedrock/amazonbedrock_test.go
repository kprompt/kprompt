package amazonbedrock

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockClient struct {
	invokeFunc func(ctx context.Context, modelID string, input []byte) ([]byte, error)
}

func (m *mockClient) InvokeModel(ctx context.Context, modelID string, input []byte) ([]byte, error) {
	return m.invokeFunc(ctx, modelID, input)
}

func TestProvider_Generate(t *testing.T) {
	ctx := context.Background()
	expectedOutput := []byte(`{"result":"ok"}`)

	tests := []struct {
		name       string
		client     Client
		modelID    string
		input      []byte
		wantOutput []byte
		wantErr    bool
	}{
		{
			name: "success",
			client: &mockClient{
				invokeFunc: func(ctx context.Context, modelID string, input []byte) ([]byte, error) {
					if modelID != "test-model" {
						t.Errorf("unexpected modelID: %s", modelID)
					}
					return expectedOutput, nil
				},
			},
			modelID:    "test-model",
			input:      []byte(`{"prompt":"hello"}`),
			wantOutput: expectedOutput,
			wantErr:    false,
		},
		{
			name:       "no client",
			client:     nil,
			modelID:    "any",
			input:      []byte(`{}`),
			wantOutput: nil,
			wantErr:    true,
		},
		{
			name: "client error",
			client: &mockClient{
				invokeFunc: func(ctx context.Context, modelID string, input []byte) ([]byte, error) {
					return nil, errors.New("invoke failed")
				},
			},
			modelID:    "fail-model",
			input:      []byte(`{}`),
			wantOutput: nil,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := New(tt.client)
			got, err := p.Generate(ctx, tt.modelID, tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("Generate() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if string(got) != string(tt.wantOutput) {
				t.Errorf("Generate() = %s, want %s", got, tt.wantOutput)
			}
		})
	}
}

func TestHTTPClient_InvokeModel(t *testing.T) {
	// This test is limited since we do not have a real endpoint.
	// We test error cases and basic setup.

	client := NewHTTPClient("", "apikey")
	_, err := client.InvokeModel(context.Background(), "model", []byte(`{}`))
	if err == nil {
		t.Error("expected error for empty endpoint")
	}

	client = NewHTTPClient("http://localhost:0", "apikey")
	// Timeout expected or connection refused.
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err = client.InvokeModel(ctx, "model", []byte(`{}`))
	if err == nil {
		t.Error("expected error for invalid endpoint")
	}
}
