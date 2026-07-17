package otel

import (
	"strings"
	"testing"
	"time"
)

func TestAnalyzeTraceHighlightsSlowChild(t *testing.T) {
	start := time.Unix(1, 0)
	report := AnalyzeTrace(Trace{
		TraceID:       "t1",
		RootService:   "payment",
		RootOperation: "POST /charge",
		Duration:      4 * time.Second,
		Spans: []Span{
			{
				SpanID:    "root",
				Service:   "payment",
				Operation: "POST /charge",
				StartTime: start,
				Duration:  4 * time.Second,
			},
			{
				SpanID:       "db",
				ParentSpanID: "root",
				Service:      "postgres",
				Operation:    "SELECT users",
				StartTime:    start.Add(200 * time.Millisecond),
				Duration:     3200 * time.Millisecond,
			},
			{
				SpanID:       "cache",
				ParentSpanID: "root",
				Service:      "redis",
				Operation:    "GET session",
				StartTime:    start.Add(50 * time.Millisecond),
				Duration:     20 * time.Millisecond,
			},
		},
	})
	if len(report.Bottlenecks) == 0 {
		t.Fatal("expected bottlenecks")
	}
	top := report.Bottlenecks[0]
	if top.Service != "postgres" {
		t.Fatalf("top=%+v", top)
	}
	if !strings.Contains(strings.ToLower(top.Message), "waited") ||
		!strings.Contains(strings.ToLower(top.Message), "postgres") {
		t.Fatalf("message=%q", top.Message)
	}
	if !strings.Contains(report.Summary, "Main bottleneck") ||
		!strings.Contains(strings.ToLower(report.Summary), "postgres") {
		t.Fatalf("summary=%q", report.Summary)
	}
}

func TestAnalyzeTraceIncludesErrors(t *testing.T) {
	report := AnalyzeTrace(Trace{
		Duration: time.Second,
		Spans: []Span{{
			SpanID:    "err",
			Service:   "payments",
			Operation: "POST /charge",
			Duration:  5 * time.Millisecond,
			Status:    "ERROR",
		}},
	})
	if len(report.Bottlenecks) != 1 {
		t.Fatalf("bottlenecks=%+v", report.Bottlenecks)
	}
	if !strings.Contains(strings.ToLower(report.Bottlenecks[0].Message), "error") ||
		!strings.Contains(strings.ToLower(report.Bottlenecks[0].Message), "payments") {
		t.Fatalf("message=%q", report.Bottlenecks[0].Message)
	}
}

func TestAnalyzeTraceSkipsTinySingleSpan(t *testing.T) {
	report := AnalyzeTrace(Trace{
		Duration: 30 * time.Millisecond,
		Spans: []Span{{
			SpanID:    "root",
			Service:   "api",
			Operation: "GET /health",
			Duration:  30 * time.Millisecond,
		}},
	})
	if len(report.Bottlenecks) != 0 {
		t.Fatalf("bottlenecks=%+v", report.Bottlenecks)
	}
}
