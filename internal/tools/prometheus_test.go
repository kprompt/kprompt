package tools

import (
	"strings"
	"testing"
)

func TestNewPrometheusClientUsesSettings(t *testing.T) {
	client, err := NewPrometheusClient(Settings{
		PrometheusEnabled: true,
		PrometheusURL:     "https://prom.example",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNewPrometheusClientRequiresConfiguration(t *testing.T) {
	_, err := NewPrometheusClient(Settings{PrometheusEnabled: true})
	if err == nil || !strings.Contains(err.Error(), EnvPrometheusURL) {
		t.Fatalf("err=%v", err)
	}
}

func TestNewPrometheusClientHonorsDisabledSetting(t *testing.T) {
	_, err := NewPrometheusClient(Settings{
		PrometheusEnabled: false,
		PrometheusURL:     "https://prom.example",
	})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("err=%v", err)
	}
}
