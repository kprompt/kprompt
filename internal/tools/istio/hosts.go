package istio

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// VirtualServiceDestinationHosts returns unique destination.host values from http/tcp routes.
func VirtualServiceDestinationHosts(vs *unstructured.Unstructured) []string {
	if vs == nil {
		return nil
	}
	var hosts []string
	seen := map[string]struct{}{}
	add := func(h string) {
		h = strings.TrimSpace(h)
		if h == "" {
			return
		}
		if _, ok := seen[h]; ok {
			return
		}
		seen[h] = struct{}{}
		hosts = append(hosts, h)
	}
	for _, block := range []string{"http", "tcp"} {
		routes, ok, _ := unstructured.NestedSlice(vs.Object, "spec", block)
		if !ok {
			continue
		}
		for _, raw := range routes {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			route, ok, _ := unstructured.NestedSlice(m, "route")
			if !ok {
				continue
			}
			for _, r := range route {
				rm, ok := r.(map[string]any)
				if !ok {
					continue
				}
				if h, ok, _ := unstructured.NestedString(rm, "destination", "host"); ok {
					add(h)
				}
			}
		}
	}
	return hosts
}

// MatchHostToService returns the Service name when host matches short or FQDN forms in ns.
func MatchHostToService(host, ns string, svcNames []string) string {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return ""
	}
	if i := strings.IndexByte(host, ':'); i >= 0 {
		host = host[:i]
	}
	for _, name := range svcNames {
		n := strings.ToLower(name)
		candidates := []string{
			n,
			n + "." + strings.ToLower(ns),
			n + "." + strings.ToLower(ns) + ".svc",
			n + "." + strings.ToLower(ns) + ".svc.cluster.local",
		}
		for _, c := range candidates {
			if host == c {
				return name
			}
		}
	}
	return ""
}

// IsCanaryVirtualService is true when any http/tcp route block has multiple destinations.
func IsCanaryVirtualService(vs *unstructured.Unstructured) bool {
	if vs == nil {
		return false
	}
	for _, block := range []string{"http", "tcp"} {
		routes, ok, _ := unstructured.NestedSlice(vs.Object, "spec", block)
		if !ok {
			continue
		}
		for _, raw := range routes {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			route, ok, _ := unstructured.NestedSlice(m, "route")
			if ok && len(route) > 1 {
				return true
			}
		}
	}
	return false
}

// IsNoMatchGVR reports discovery errors when a CRD/GVR is not installed.
func IsNoMatchGVR(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "no matches for kind") ||
		strings.Contains(msg, "could not find the requested resource") ||
		strings.Contains(msg, "the server could not find the requested resource")
}
