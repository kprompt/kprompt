package doctor

import (
	"net"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/config"
)

func TestIsSensitiveIP(t *testing.T) {
	cases := []struct {
		ip   string
		want bool
	}{
		{"127.0.0.1", true},
		{"::1", true},
		{"10.0.0.5", true},
		{"192.168.1.1", true},
		{"172.16.0.1", true},
		{"169.254.1.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
	}
	for _, tc := range cases {
		if got := isSensitiveIP(net.ParseIP(tc.ip)); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.ip, got, tc.want)
		}
	}
}

func TestHostFromEndpoint(t *testing.T) {
	h, err := hostFromEndpoint("https://llm.example.com:8443/v1")
	if err != nil || h != "llm.example.com" {
		t.Fatalf("got %q %v", h, err)
	}
	h, err = hostFromEndpoint("10.0.0.9:9090")
	if err != nil || h != "10.0.0.9" {
		t.Fatalf("otel-style got %q %v", h, err)
	}
}

func TestCheckEndpointURLsSkipEmpty(t *testing.T) {
	c := checkEndpointURLs(config.File{})
	if c.Status != Skip || c.ID != "endpoint-urls" {
		t.Fatalf("%+v", c)
	}
}

func TestCheckEndpointURLsWarnPrivateLiteral(t *testing.T) {
	orig := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		if host == "prom.internal.example" {
			return []net.IP{net.ParseIP("192.168.50.2")}, nil
		}
		return orig(host)
	}
	t.Cleanup(func() { lookupIP = orig })

	c := checkEndpointURLs(config.File{
		BaseURL: "http://10.1.2.3:8080/v1",
		Tools: config.ToolsFile{
			Prometheus: config.PrometheusTool{URL: "https://prom.internal.example/"},
		},
	})
	if c.Status != Warn {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(c.Detail, "llm.base_url") || !strings.Contains(c.Detail, "tools.prometheus.url") {
		t.Fatalf("detail=%q", c.Detail)
	}
	if !strings.Contains(c.Hint, "Decision A") {
		t.Fatalf("hint=%q", c.Hint)
	}
}

func TestCheckEndpointURLsPassPublic(t *testing.T) {
	orig := lookupIP
	lookupIP = func(host string) ([]net.IP, error) {
		return []net.IP{net.ParseIP("1.1.1.1")}, nil
	}
	t.Cleanup(func() { lookupIP = orig })

	c := checkEndpointURLs(config.File{
		BaseURL: "https://api.openai.com/v1",
	})
	if c.Status != Pass {
		t.Fatalf("%+v", c)
	}
}
