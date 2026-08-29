package doctor

import (
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/kprompt/kprompt/internal/config"
)

// lookupIP resolves a hostname. Overridden in tests.
var lookupIP = net.LookupIP

// namedEndpoint is a configured operator URL checked by the private-range advisory.
type namedEndpoint struct {
	Name string
	Raw  string
}

func collectConfiguredEndpoints(file config.File) []namedEndpoint {
	r := config.Merge(file, "", "", "", "", false, "")
	var out []namedEndpoint
	if u := strings.TrimSpace(r.BaseURL); u != "" {
		out = append(out, namedEndpoint{Name: "llm.base_url", Raw: u})
	}
	if u := strings.TrimSpace(file.Tools.Prometheus.URL); u != "" {
		out = append(out, namedEndpoint{Name: "tools.prometheus.url", Raw: u})
	}
	if u := strings.TrimSpace(file.Tools.Grafana.URL); u != "" {
		out = append(out, namedEndpoint{Name: "tools.grafana.url", Raw: u})
	}
	if u := strings.TrimSpace(file.Tools.OTel.Endpoint); u != "" {
		out = append(out, namedEndpoint{Name: "tools.otel.endpoint", Raw: u})
	}
	return out
}

func hostFromEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", fmt.Errorf("empty")
	}
	if !strings.Contains(raw, "://") {
		raw = "http://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	host := u.Hostname()
	if host == "" {
		return "", fmt.Errorf("no host in %q", raw)
	}
	return host, nil
}

// isSensitiveIP reports RFC-1918 / ULA / loopback / link-local addresses.
// Decision A still allows these; doctor only warns.
func isSensitiveIP(ip net.IP) bool {
	if ip == nil {
		return false
	}
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
}

func checkEndpointURLs(file config.File) Check {
	c := Check{
		ID:       "endpoint-urls",
		Name:     "Operator endpoint URLs",
		Required: false,
	}
	eps := collectConfiguredEndpoints(file)
	if len(eps) == 0 {
		c.Status = Skip
		c.Detail = "no base_url / prometheus / grafana / otel endpoints configured"
		c.Hint = "When set, doctor warns if they resolve to private/link-local/loopback — docs/security/operator-endpoint-hardening.md (SEC-007)"
		return c
	}

	var sensitive []string
	var unresolved []string
	var okCount int
	for _, ep := range eps {
		host, err := hostFromEndpoint(ep.Raw)
		if err != nil {
			unresolved = append(unresolved, ep.Name+": parse error")
			continue
		}
		// Literal IP in the URL — no DNS needed.
		if ip := net.ParseIP(host); ip != nil {
			if isSensitiveIP(ip) {
				sensitive = append(sensitive, fmt.Sprintf("%s→%s", ep.Name, ip.String()))
			} else {
				okCount++
			}
			continue
		}
		ips, err := lookupIP(host)
		if err != nil || len(ips) == 0 {
			unresolved = append(unresolved, ep.Name)
			continue
		}
		hit := false
		for _, ip := range ips {
			if isSensitiveIP(ip) {
				sensitive = append(sensitive, fmt.Sprintf("%s→%s", ep.Name, ip.String()))
				hit = true
				break
			}
		}
		if !hit {
			okCount++
		}
	}

	parts := []string{fmt.Sprintf("%d configured", len(eps))}
	if okCount > 0 {
		parts = append(parts, fmt.Sprintf("%d public/resolvable", okCount))
	}
	if len(unresolved) > 0 {
		parts = append(parts, "unresolved="+strings.Join(unresolved, ","))
	}

	if len(sensitive) == 0 {
		c.Status = Pass
		c.Detail = strings.Join(parts, " · ")
		if len(unresolved) > 0 {
			c.Status = Warn
			c.Detail += " · could not resolve some hosts"
			c.Hint = "Check DNS / spelling; private ranges are OK when operator-owned — pin via NetworkPolicy CIDRs"
		}
		return c
	}

	c.Status = Warn
	c.Detail = strings.Join(parts, " · ") + " · private/link-local/loopback: " + strings.Join(sensitive, ", ")
	c.Hint = "Advisory only (SEC-007 Decision A) — ensure NetworkPolicy/CNI allowlists match; see docs/security/operator-endpoint-hardening.md"
	return c
}
