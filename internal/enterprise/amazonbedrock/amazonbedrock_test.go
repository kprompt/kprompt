package amazonbedrock

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("https://bedrock.example.com", "testkey")
	if c.Endpoint != "https://bedrock.example.com" {
		t.Errorf("expected endpoint to be set")
	}
	if c.APIKey != "testkey" {
		t.Errorf("expected APIKey to be set")
	}
	if c.HTTPClient == nil {
		t.Errorf("expected HTTPClient to be set")
	}
}

func TestGenerateTextErrors(t *testing.T) {
	c := NewClient("", "")
	_, err := c.GenerateText(context.Background(), "hello")
	if err == nil {
		t.Errorf("expected error for empty endpoint")
	}

	c = NewClient("https://bedrock.example.com", "")
	_, err = c.GenerateText(context.Background(), "hello")
	if err == nil {
		t.Errorf("expected error for empty API key")
	}

	c = NewClient("https://bedrock.example.com", "key")
	_, err = c.GenerateText(context.Background(), "")
	if err == nil {
		t.Errorf("expected error for empty prompt")
	}
}

func TestGenerateTextSuccess(t *testing.T) {
	c := NewClient("https://bedrock.example.com", "key")
	resp, err := c.GenerateText(context.Background(), "test prompt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	expected := "simulated response for prompt: test prompt"
	if resp != expected {
		t.Errorf("expected %q, got %q", expected, resp)
	}
}
