package tools

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/config"
	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
)

func TestNewGrafanaClientUsesSettings(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization=%q", r.Header.Get("Authorization"))
		}
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()

	client, err := NewGrafanaClient(Settings{
		GrafanaEnabled: true,
		GrafanaURL:     server.URL,
		GrafanaAPIKey:  "secret",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ListDashboards(
		context.Background(),
		toolgrafana.SearchRequest{},
	); err != nil {
		t.Fatal(err)
	}
}

func TestNewGrafanaClientRequiresConfiguration(t *testing.T) {
	_, err := NewGrafanaClient(Settings{GrafanaEnabled: true})
	if err == nil || !strings.Contains(err.Error(), EnvGrafanaURL) {
		t.Fatalf("err=%v", err)
	}
}

func TestNewGrafanaClientHonorsDisabledSetting(t *testing.T) {
	_, err := NewGrafanaClient(Settings{
		GrafanaEnabled: false,
		GrafanaURL:     "https://grafana.example",
	})
	if err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("err=%v", err)
	}
}

func TestLoadSettingsGrafanaAPIKeyFromEnv(t *testing.T) {
	t.Setenv(EnvGrafanaAPIKey, "secret")
	settings := LoadSettings(config.File{
		Tools: config.ToolsFile{
			Grafana: config.GrafanaTool{URL: "https://grafana.example"},
		},
	})
	if settings.GrafanaAPIKey != "secret" {
		t.Fatalf("API key=%q", settings.GrafanaAPIKey)
	}
}
