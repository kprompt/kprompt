package otel

import (
	"context"
	"errors"
	"testing"
	"time"
)

type querierFunc func(context.Context, SearchRequest) ([]Trace, error)

func (f querierFunc) SearchTraces(ctx context.Context, req SearchRequest) ([]Trace, error) {
	return f(ctx, req)
}

func TestLatestTraceSelectsNewest(t *testing.T) {
	old := Trace{TraceID: "old", StartTime: time.Unix(1, 0)}
	newest := Trace{TraceID: "new", StartTime: time.Unix(2, 0)}
	got, err := LatestTrace(
		context.Background(),
		querierFunc(func(context.Context, SearchRequest) ([]Trace, error) {
			return []Trace{old, newest}, nil
		}),
		SearchRequest{Service: "payment"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.TraceID != "new" {
		t.Fatalf("trace=%s", got.TraceID)
	}
}

func TestLatestTraceReportsEmptySearch(t *testing.T) {
	_, err := LatestTrace(
		context.Background(),
		querierFunc(func(context.Context, SearchRequest) ([]Trace, error) {
			return nil, nil
		}),
		SearchRequest{Service: "payment"},
	)
	if !errors.Is(err, ErrTraceNotFound) {
		t.Fatalf("err=%v", err)
	}
}

func TestWalkSpansOrdersParentsBeforeChildrenAndKeepsOrphans(t *testing.T) {
	start := time.Unix(1, 0)
	rows := WalkSpans(Trace{Spans: []Span{
		{SpanID: "child", ParentSpanID: "root", Operation: "child", StartTime: start.Add(time.Second)},
		{SpanID: "orphan", ParentSpanID: "missing", Operation: "orphan", StartTime: start.Add(2 * time.Second)},
		{SpanID: "root", Operation: "root", StartTime: start},
	}})
	if len(rows) != 3 {
		t.Fatalf("rows=%+v", rows)
	}
	if rows[0].Span.SpanID != "root" || rows[0].Depth != 0 {
		t.Fatalf("root=%+v", rows[0])
	}
	if rows[1].Span.SpanID != "child" || rows[1].Depth != 1 {
		t.Fatalf("child=%+v", rows[1])
	}
	if rows[2].Span.SpanID != "orphan" || rows[2].Depth != 0 {
		t.Fatalf("orphan=%+v", rows[2])
	}
}
