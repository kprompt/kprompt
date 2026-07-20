package graph

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
)

const (
	SourceOTel     = "otel"
	EdgeCalls      = "calls"
	maxOTelServices = 20
	maxOTelTraces   = 5
	defaultOTelWindow = time.Hour
)

// EnrichFromOTel merges recent trace-derived service→service edges onto a K8s graph.
// Missing or failing OTel never fails the report — notes record degradation (T-060).
func EnrichFromOTel(
	ctx context.Context,
	querier toolotel.Querier,
	rep *Report,
	window time.Duration,
) {
	if rep == nil {
		return
	}
	if querier == nil {
		rep.Notes = append(rep.Notes, "OTel unavailable: not configured; graph is Kubernetes-only")
		refreshSummary(rep)
		return
	}
	if window <= 0 {
		window = defaultOTelWindow
	}

	services := serviceNodes(*rep)
	if len(services) == 0 {
		rep.Notes = append(rep.Notes, "OTel enrichment skipped: no Service nodes in graph")
		refreshSummary(rep)
		return
	}
	if len(services) > maxOTelServices {
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"OTel enrichment limited to first %d of %d Service nodes",
			maxOTelServices, len(services),
		))
		services = services[:maxOTelServices]
	}

	index := indexServicesByName(*rep)
	end := time.Now().UTC()
	start := end.Add(-window)
	var otelEdges []Edge
	var searchErrs int
	var unmatched int

	for _, svc := range services {
		select {
		case <-ctx.Done():
			rep.Notes = append(rep.Notes, "OTel enrichment canceled")
			rep.Edges = append(rep.Edges, otelEdges...)
			dedupeEdges(rep)
			refreshSummary(rep)
			return
		default:
		}
		traces, err := querier.SearchTraces(ctx, toolotel.SearchRequest{
			Service: svc.Name,
			Start:   start,
			End:     end,
			Limit:   maxOTelTraces,
		})
		if err != nil {
			searchErrs++
			continue
		}
		for _, tr := range traces {
			byID := make(map[string]toolotel.Span, len(tr.Spans))
			for _, sp := range tr.Spans {
				byID[sp.SpanID] = sp
			}
			for _, sp := range tr.Spans {
				if sp.ParentSpanID == "" {
					continue
				}
				parent, ok := byID[sp.ParentSpanID]
				if !ok {
					continue
				}
				fromSvc := strings.TrimSpace(parent.Service)
				toSvc := strings.TrimSpace(sp.Service)
				if fromSvc == "" || toSvc == "" || strings.EqualFold(fromSvc, toSvc) {
					continue
				}
				fromID, fok := resolveServiceNode(index, fromSvc, rep.Namespace)
				toID, tok := resolveServiceNode(index, toSvc, rep.Namespace)
				if !fok || !tok {
					unmatched++
					continue
				}
				detail := sp.Operation
				if detail == "" {
					detail = tr.TraceID
				}
				otelEdges = append(otelEdges, Edge{
					From:   fromID,
					To:     toID,
					Type:   EdgeCalls,
					Detail: detail,
					Source: SourceOTel,
				})
			}
		}
	}

	if searchErrs > 0 {
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"OTel search failed for %d/%d services (partial enrichment)",
			searchErrs, len(services),
		))
	}
	if unmatched > 0 {
		rep.Notes = append(rep.Notes, fmt.Sprintf(
			"OTel skipped %d span edges with no matching Service node",
			unmatched,
		))
	}
	if len(otelEdges) == 0 && searchErrs == 0 {
		rep.Notes = append(rep.Notes, "OTel returned no cross-service call edges in the search window")
	}

	rep.Edges = append(rep.Edges, otelEdges...)
	dedupeEdges(rep)
	refreshSummary(rep)
}

func serviceNodes(rep Report) []Node {
	out := make([]Node, 0)
	for _, n := range rep.Nodes {
		if n.Kind == NodeService {
			out = append(out, n)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Namespace != out[j].Namespace {
			return out[i].Namespace < out[j].Namespace
		}
		return out[i].Name < out[j].Name
	})
	return out
}

func indexServicesByName(rep Report) map[string][]Node {
	out := map[string][]Node{}
	for _, n := range rep.Nodes {
		if n.Kind != NodeService {
			continue
		}
		key := strings.ToLower(n.Name)
		out[key] = append(out[key], n)
	}
	return out
}

func resolveServiceNode(index map[string][]Node, serviceName, preferNS string) (string, bool) {
	cands := index[strings.ToLower(strings.TrimSpace(serviceName))]
	if len(cands) == 0 {
		return "", false
	}
	if preferNS != "" {
		for _, c := range cands {
			if c.Namespace == preferNS {
				return c.ID, true
			}
		}
	}
	if len(cands) == 1 {
		return cands[0].ID, true
	}
	// Ambiguous across namespaces — pick lexicographically stable ID.
	sort.Slice(cands, func(i, j int) bool { return cands[i].ID < cands[j].ID })
	return cands[0].ID, true
}

func dedupeEdges(rep *Report) {
	if rep == nil {
		return
	}
	seen := map[string]struct{}{}
	unique := make([]Edge, 0, len(rep.Edges))
	for _, e := range rep.Edges {
		k := e.From + "|" + e.To + "|" + e.Type + "|" + e.Source
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		unique = append(unique, e)
	}
	sort.Slice(unique, func(i, j int) bool {
		if unique[i].Source != unique[j].Source {
			return unique[i].Source < unique[j].Source
		}
		if unique[i].From != unique[j].From {
			return unique[i].From < unique[j].From
		}
		return unique[i].To < unique[j].To
	})
	rep.Edges = unique
}

func refreshSummary(rep *Report) {
	if rep == nil {
		return
	}
	svcN, podN, npN, otelN, k8sN := 0, 0, 0, 0, 0
	for _, n := range rep.Nodes {
		switch n.Kind {
		case NodeService:
			svcN++
		case NodePod:
			podN++
		case NodeNetworkPolicy:
			npN++
		}
	}
	for _, e := range rep.Edges {
		switch e.Source {
		case SourceOTel:
			otelN++
		default:
			k8sN++
		}
	}
	scopeLabel := "cluster"
	if rep.Scope == ScopeNamespace && rep.Namespace != "" {
		scopeLabel = fmt.Sprintf("namespace %q", rep.Namespace)
	}
	rep.Summary = fmt.Sprintf(
		"Service dependency graph for %s: %d services, %d pods, %d network policies, %d k8s edges, %d otel call edges.",
		scopeLabel, svcN, podN, npN, k8sN, otelN,
	)
}
