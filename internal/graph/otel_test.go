package graph

import (
	"context"
	"strings"
	"testing"
	"time"

	toolotel "github.com/kprompt/kprompt/internal/tools/otel"
)

type otelQuerierFunc func(context.Context, toolotel.SearchRequest) ([]toolotel.Trace, error)

func (f otelQuerierFunc) SearchTraces(ctx context.Context, req toolotel.SearchRequest) ([]toolotel.Trace, error) {
	return f(ctx, req)
}

func TestEnrichFromOTelAddsCallEdges(t *testing.T) {
	rep := Report{
		Type:  "service-graph",
		Scope: ScopeNamespace,
		Namespace: "prod",
		Nodes: []Node{
			{ID: "prod/Service/api", Kind: NodeService, Name: "api", Namespace: "prod"},
			{ID: "prod/Service/db", Kind: NodeService, Name: "db", Namespace: "prod"},
			{ID: "prod/Pod/api-1", Kind: NodePod, Name: "api-1", Namespace: "prod"},
		},
		Edges: []Edge{{
			From: "prod/Service/api", To: "prod/Pod/api-1",
			Type: EdgeRoutes, Source: SourceKubernetes,
		}},
	}
	q := otelQuerierFunc(func(_ context.Context, req toolotel.SearchRequest) ([]toolotel.Trace, error) {
		if req.Service != "api" && req.Service != "db" {
			return nil, nil
		}
		return []toolotel.Trace{{
			TraceID: "t1",
			Spans: []toolotel.Span{
				{SpanID: "1", Service: "api", Operation: "GET /", StartTime: time.Now()},
				{SpanID: "2", ParentSpanID: "1", Service: "db", Operation: "SELECT", StartTime: time.Now()},
			},
		}}, nil
	})
	EnrichFromOTel(context.Background(), q, &rep, time.Hour)
	var found bool
	for _, e := range rep.Edges {
		if e.Source == SourceOTel && e.Type == EdgeCalls &&
			e.From == "prod/Service/api" && e.To == "prod/Service/db" {
			found = true
		}
	}
	if !found {
		t.Fatalf("edges=%+v", rep.Edges)
	}
	if !strings.Contains(rep.Summary, "otel call edges") {
		t.Fatalf("summary=%s", rep.Summary)
	}
}

func TestEnrichFromOTelMissingQuerierNotes(t *testing.T) {
	rep := Report{
		Nodes: []Node{{ID: "default/Service/api", Kind: NodeService, Name: "api", Namespace: "default"}},
	}
	EnrichFromOTel(context.Background(), nil, &rep, time.Hour)
	if len(rep.Notes) == 0 || !strings.Contains(rep.Notes[0], "OTel unavailable") {
		t.Fatalf("notes=%v", rep.Notes)
	}
}

func TestEnrichFromOTelDoesNotFailOnSearchError(t *testing.T) {
	rep := Report{
		Nodes: []Node{{ID: "default/Service/api", Kind: NodeService, Name: "api", Namespace: "default"}},
	}
	q := otelQuerierFunc(func(context.Context, toolotel.SearchRequest) ([]toolotel.Trace, error) {
		return nil, context.DeadlineExceeded
	})
	EnrichFromOTel(context.Background(), q, &rep, time.Hour)
	if len(rep.Notes) == 0 {
		t.Fatal("expected partial failure note")
	}
}
