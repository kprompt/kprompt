package istio

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

// VirtualServiceGVR is the namespaced resource for Istio VirtualServices.
var VirtualServiceGVR = schema.GroupVersionResource{
	Group:    VirtualServiceGroup,
	Version:  "v1beta1",
	Resource: "virtualservices",
}

// TrafficRequest configures a read-only VirtualService traffic summary.
type TrafficRequest struct {
	Namespace string // empty → all namespaces
	Name      string // optional single VirtualService
}

// DestinationSplit is one weighted destination in an HTTP/TCP route.
type DestinationSplit struct {
	Host   string `json:"host"`
	Subset string `json:"subset,omitempty"`
	Port   int64  `json:"port,omitempty"`
	Weight int64  `json:"weight"`
}

// RouteSummary describes one route block on a VirtualService.
type RouteSummary struct {
	Match  string             `json:"match,omitempty"`
	Splits []DestinationSplit `json:"splits"`
}

// VirtualServiceSummary is a compact view of one VirtualService.
type VirtualServiceSummary struct {
	Name      string         `json:"name"`
	Namespace string         `json:"namespace"`
	Hosts     []string       `json:"hosts,omitempty"`
	Gateways  []string       `json:"gateways,omitempty"`
	Routes    []RouteSummary `json:"routes,omitempty"`
	Canary    bool           `json:"canary"`
}

// TrafficReport is the stable human + JSON contract for Istio traffic (T-041).
type TrafficReport struct {
	Type            string                  `json:"type"`
	Scope           string                  `json:"scope"`
	Namespace       string                  `json:"namespace,omitempty"`
	Summary         string                  `json:"summary"`
	VirtualServices []VirtualServiceSummary `json:"virtualServices"`
	Notes           []string                `json:"notes,omitempty"`
}

// SummarizeTraffic lists VirtualServices and narrates hosts, routes, and canary weights.
func SummarizeTraffic(ctx context.Context, cfg *rest.Config, req TrafficRequest) (TrafficReport, error) {
	if cfg == nil {
		return TrafficReport{}, fmt.Errorf("istio traffic: rest config is nil")
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return TrafficReport{}, fmt.Errorf("istio dynamic client: %w", err)
	}
	return SummarizeTrafficWithClient(ctx, dc, req)
}

// SummarizeTrafficWithClient builds a traffic report using an injected dynamic client.
func SummarizeTrafficWithClient(ctx context.Context, dc dynamic.Interface, req TrafficRequest) (TrafficReport, error) {
	ns := strings.TrimSpace(req.Namespace)
	name := strings.TrimSpace(req.Name)
	scope := "cluster"
	if ns != "" {
		scope = "namespace"
	}
	rep := TrafficReport{
		Type:            "istio-traffic",
		Scope:           scope,
		Namespace:       ns,
		VirtualServices: make([]VirtualServiceSummary, 0, 8),
	}
	limit := int64(cluster.DefaultReadLimit)
	gvr := VirtualServiceGVR

	var objs []unstructured.Unstructured
	if name != "" {
		if ns == "" {
			return TrafficReport{}, fmt.Errorf("virtualservice %q requires a namespace", name)
		}
		obj, err := dc.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				rep.Notes = append(rep.Notes, fmt.Sprintf("VirtualService/%s not found in %s", name, ns))
				rep.Summary = "No matching VirtualService"
				return rep, nil
			}
			return TrafficReport{}, fmt.Errorf("get virtualservice: %w", err)
		}
		objs = append(objs, *obj)
	} else {
		var list *unstructured.UnstructuredList
		var err error
		if ns == "" {
			list, err = dc.Resource(gvr).List(ctx, metav1.ListOptions{Limit: limit})
		} else {
			list, err = dc.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{Limit: limit})
		}
		if err != nil {
			if apierrors.IsForbidden(err) {
				rep.Notes = append(rep.Notes, fmt.Sprintf("skipped VirtualServices: %v", err))
				rep.Summary = "Unable to list VirtualServices (RBAC)"
				return rep, nil
			}
			return TrafficReport{}, fmt.Errorf("list virtualservices: %w", err)
		}
		objs = list.Items
	}

	canaryCount := 0
	for i := range objs {
		sum := summarizeVirtualService(&objs[i])
		if sum.Canary {
			canaryCount++
		}
		rep.VirtualServices = append(rep.VirtualServices, sum)
	}
	sort.Slice(rep.VirtualServices, func(i, j int) bool {
		if rep.VirtualServices[i].Namespace != rep.VirtualServices[j].Namespace {
			return rep.VirtualServices[i].Namespace < rep.VirtualServices[j].Namespace
		}
		return rep.VirtualServices[i].Name < rep.VirtualServices[j].Name
	})

	switch {
	case len(rep.VirtualServices) == 0:
		rep.Summary = "No VirtualServices found"
	case canaryCount > 0:
		rep.Summary = fmt.Sprintf("%d VirtualService(s), %d with canary/weighted splits", len(rep.VirtualServices), canaryCount)
	default:
		rep.Summary = fmt.Sprintf("%d VirtualService(s), no weighted canary splits", len(rep.VirtualServices))
	}
	return rep, nil
}

func summarizeVirtualService(obj *unstructured.Unstructured) VirtualServiceSummary {
	sum := VirtualServiceSummary{
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if hosts, ok, _ := unstructured.NestedStringSlice(obj.Object, "spec", "hosts"); ok {
		sum.Hosts = hosts
	}
	if gws, ok, _ := unstructured.NestedStringSlice(obj.Object, "spec", "gateways"); ok {
		sum.Gateways = gws
	}

	if httpRoutes, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "http"); ok {
		for _, raw := range httpRoutes {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			route := RouteSummary{Match: matchLabel(m)}
			route.Splits = destinationSplits(m)
			if len(route.Splits) > 1 {
				sum.Canary = true
			}
			sum.Routes = append(sum.Routes, route)
		}
	}
	if tcpRoutes, ok, _ := unstructured.NestedSlice(obj.Object, "spec", "tcp"); ok {
		for _, raw := range tcpRoutes {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			route := RouteSummary{Match: "tcp"}
			route.Splits = destinationSplits(m)
			if len(route.Splits) > 1 {
				sum.Canary = true
			}
			sum.Routes = append(sum.Routes, route)
		}
	}
	return sum
}

func matchLabel(route map[string]any) string {
	matches, ok, _ := unstructured.NestedSlice(route, "match")
	if !ok || len(matches) == 0 {
		return ""
	}
	m, ok := matches[0].(map[string]any)
	if !ok {
		return ""
	}
	if uri, ok, _ := unstructured.NestedString(m, "uri", "prefix"); ok && uri != "" {
		return "uri:" + uri
	}
	if uri, ok, _ := unstructured.NestedString(m, "uri", "exact"); ok && uri != "" {
		return "uri:" + uri
	}
	if method, ok, _ := unstructured.NestedString(m, "method", "exact"); ok && method != "" {
		return "method:" + method
	}
	return "matched"
}

func destinationSplits(route map[string]any) []DestinationSplit {
	rawRoute, ok, _ := unstructured.NestedSlice(route, "route")
	if !ok {
		return nil
	}
	out := make([]DestinationSplit, 0, len(rawRoute))
	for _, raw := range rawRoute {
		m, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		split := DestinationSplit{Weight: 100}
		if w, ok := int64Field(m, "weight"); ok {
			split.Weight = w
		}
		dest, ok, _ := unstructured.NestedMap(m, "destination")
		if ok {
			if h, ok, _ := unstructured.NestedString(dest, "host"); ok {
				split.Host = h
			}
			if s, ok, _ := unstructured.NestedString(dest, "subset"); ok {
				split.Subset = s
			}
			if p, ok := int64Field(dest, "port", "number"); ok {
				split.Port = p
			}
		}
		out = append(out, split)
	}
	return out
}

func int64Field(obj map[string]any, fields ...string) (int64, bool) {
	if len(fields) == 0 {
		return 0, false
	}
	cur := any(obj)
	for _, f := range fields[:len(fields)-1] {
		m, ok := cur.(map[string]any)
		if !ok {
			return 0, false
		}
		cur, ok = m[f]
		if !ok {
			return 0, false
		}
	}
	m, ok := cur.(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := m[fields[len(fields)-1]]
	if !ok {
		return 0, false
	}
	switch n := v.(type) {
	case int64:
		return n, true
	case int32:
		return int64(n), true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	case float32:
		return int64(n), true
	default:
		return 0, false
	}
}
