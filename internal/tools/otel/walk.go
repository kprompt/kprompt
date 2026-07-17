package otel

import (
	"context"
	"errors"
	"fmt"
	"sort"
)

// ErrTraceNotFound reports a valid search with no matching traces.
var ErrTraceNotFound = errors.New("no matching traces found")

// Querier is the read-only capability needed by the trace intent.
type Querier interface {
	SearchTraces(context.Context, SearchRequest) ([]Trace, error)
}

// LatestTrace searches and selects the newest matching trace.
func LatestTrace(ctx context.Context, querier Querier, req SearchRequest) (Trace, error) {
	if querier == nil {
		return Trace{}, fmt.Errorf("trace querier is nil")
	}
	traces, err := querier.SearchTraces(ctx, req)
	if err != nil {
		return Trace{}, err
	}
	if len(traces) == 0 {
		return Trace{}, ErrTraceNotFound
	}
	sort.SliceStable(traces, func(i, j int) bool {
		return traces[i].StartTime.After(traces[j].StartTime)
	})
	return traces[0], nil
}

// SpanRow is one span with its depth in a parent-before-child tree walk.
type SpanRow struct {
	Depth int  `json:"depth"`
	Span  Span `json:"span"`
}

// WalkSpans returns a deterministic tree walk and safely handles orphan/cyclic spans.
func WalkSpans(trace Trace) []SpanRow {
	spans := append([]Span(nil), trace.Spans...)
	sort.SliceStable(spans, func(i, j int) bool {
		return spans[i].StartTime.Before(spans[j].StartTime)
	})

	known := make(map[string]bool, len(spans))
	for _, span := range spans {
		known[span.SpanID] = true
	}
	children := make(map[string][]int, len(spans))
	var roots []int
	for index, span := range spans {
		if span.ParentSpanID == "" || !known[span.ParentSpanID] {
			roots = append(roots, index)
			continue
		}
		children[span.ParentSpanID] = append(children[span.ParentSpanID], index)
	}

	rows := make([]SpanRow, 0, len(spans))
	visited := make(map[int]bool, len(spans))
	var walk func(int, int)
	walk = func(index, depth int) {
		if visited[index] {
			return
		}
		visited[index] = true
		rows = append(rows, SpanRow{Depth: depth, Span: spans[index]})
		for _, child := range children[spans[index].SpanID] {
			walk(child, depth+1)
		}
	}
	for _, root := range roots {
		walk(root, 0)
	}
	for index := range spans {
		walk(index, 0)
	}
	return rows
}
