package tools

import (
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/config"
)

func TestNewOTelClientUsesSettings(t *testing.T) {
	client, err := NewOTelClient(Settings{
		OTelEnabled:  true,
		OTelEndpoint: "https://tempo.example",
		OTelBackend:  "tempo",
	})
	if err != nil {
		t.Fatal(err)
	}
	if client == nil {
		t.Fatal("expected client")
	}
}

func TestNewOTelClientRequiresEndpoint(t *testing.T) {
	_, err := NewOTelClient(Settings{OTelEnabled: true, OTelBackend: "auto"})
	if err == nil || !strings.Contains(err.Error(), EnvOTelEndpoint) {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadSettingsOTelBackendEnvOverride(t *testing.T) {
	t.Setenv(EnvOTelBackend, "tempo")
	settings := LoadSettings(config.File{
		Tools: config.ToolsFile{
			OTel: config.OTelTool{Backend: "jaeger"},
		},
	})
	if settings.OTelBackend != "tempo" {
		t.Fatalf("backend=%q", settings.OTelBackend)
	}
}
