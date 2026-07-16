package config

import "testing"

func TestMergeCarriesToolConfiguration(t *testing.T) {
	file := File{
		Tools: ToolsFile{
			Prometheus: PrometheusTool{URL: "https://prom.example"},
		},
	}
	resolved := Merge(file, "ollama", "", "", "", false, "why is api slow")
	if resolved.Tools.Prometheus.URL != "https://prom.example" {
		t.Fatalf("prometheus URL=%q", resolved.Tools.Prometheus.URL)
	}
}
