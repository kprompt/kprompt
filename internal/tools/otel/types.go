package otel

import "time"

// Backend identifies a supported trace query API.
type Backend string

const (
	BackendAuto   Backend = "auto"
	BackendJaeger Backend = "jaeger"
	BackendTempo  Backend = "tempo"
)

// SearchRequest filters recent traces by service and optional operation.
type SearchRequest struct {
	Service   string
	Operation string
	Start     time.Time
	End       time.Time
	Limit     int
}

// Trace is a backend-neutral distributed trace.
type Trace struct {
	TraceID       string        `json:"traceId"`
	RootService   string        `json:"rootService,omitempty"`
	RootOperation string        `json:"rootOperation,omitempty"`
	StartTime     time.Time     `json:"startTime,omitempty"`
	Duration      time.Duration `json:"duration"`
	Spans         []Span        `json:"spans"`
}

// Span is a backend-neutral trace span.
type Span struct {
	TraceID      string            `json:"traceId"`
	SpanID       string            `json:"spanId"`
	ParentSpanID string            `json:"parentSpanId,omitempty"`
	Service      string            `json:"service,omitempty"`
	Operation    string            `json:"operation"`
	StartTime    time.Time         `json:"startTime,omitempty"`
	Duration     time.Duration     `json:"duration"`
	Status       string            `json:"status,omitempty"`
	Attributes   map[string]string `json:"attributes,omitempty"`
}
