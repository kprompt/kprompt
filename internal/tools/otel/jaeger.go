package otel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type jaegerEnvelope struct {
	Data   []jaegerTrace `json:"data"`
	Errors []any         `json:"errors"`
}

type jaegerTrace struct {
	TraceID   string                   `json:"traceID"`
	Spans     []jaegerSpan             `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
}

type jaegerSpan struct {
	TraceID       string            `json:"traceID"`
	SpanID        string            `json:"spanID"`
	OperationName string            `json:"operationName"`
	References    []jaegerReference `json:"references"`
	StartTime     int64             `json:"startTime"`
	Duration      int64             `json:"duration"`
	Tags          []jaegerTag       `json:"tags"`
	ProcessID     string            `json:"processID"`
	Process       *jaegerProcess    `json:"process,omitempty"`
}

type jaegerReference struct {
	RefType string `json:"refType"`
	SpanID  string `json:"spanID"`
}

type jaegerProcess struct {
	ServiceName string      `json:"serviceName"`
	Tags        []jaegerTag `json:"tags"`
}

type jaegerTag struct {
	Key   string `json:"key"`
	Type  string `json:"type"`
	Value any    `json:"value"`
}

func (c *Client) searchJaeger(ctx context.Context, req SearchRequest) ([]Trace, error) {
	query := url.Values{
		"service": {req.Service},
		"start":   {strconv.FormatInt(req.Start.UnixMicro(), 10)},
		"end":     {strconv.FormatInt(req.End.UnixMicro(), 10)},
		"limit":   {strconv.Itoa(req.Limit)},
	}
	if req.Operation != "" {
		query.Set("operation", req.Operation)
	}
	body, err := c.get(ctx, "/api/traces", query)
	if err != nil {
		return nil, err
	}
	return decodeJaegerTraces(body)
}

func decodeJaegerTraces(body []byte) ([]Trace, error) {
	var envelope jaegerEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, fmt.Errorf("decode Jaeger response: %w", err)
	}
	if envelope.Data == nil {
		return nil, fmt.Errorf("decode Jaeger response: missing data")
	}
	traces := make([]Trace, 0, len(envelope.Data))
	for _, raw := range envelope.Data {
		traces = append(traces, normalizeJaegerTrace(raw))
	}
	return traces, nil
}

func decodeJaegerTrace(body []byte) (Trace, error) {
	traces, err := decodeJaegerTraces(body)
	if err != nil {
		return Trace{}, err
	}
	if len(traces) == 0 {
		return Trace{}, fmt.Errorf("Jaeger trace response is empty")
	}
	return traces[0], nil
}

func normalizeJaegerTrace(raw jaegerTrace) Trace {
	trace := Trace{
		TraceID: raw.TraceID,
		Spans:   make([]Span, 0, len(raw.Spans)),
	}
	var traceEnd time.Time
	for _, rawSpan := range raw.Spans {
		process := rawSpan.Process
		if process == nil {
			if value, ok := raw.Processes[rawSpan.ProcessID]; ok {
				process = &value
			}
		}
		service := ""
		attributes := tagsToAttributes(rawSpan.Tags)
		if process != nil {
			service = process.ServiceName
			for key, value := range tagsToAttributes(process.Tags) {
				if _, exists := attributes[key]; !exists {
					attributes[key] = value
				}
			}
		}
		parentID := ""
		for _, reference := range rawSpan.References {
			if strings.EqualFold(reference.RefType, "CHILD_OF") {
				parentID = reference.SpanID
				break
			}
		}
		start := time.UnixMicro(rawSpan.StartTime).UTC()
		duration := time.Duration(rawSpan.Duration) * time.Microsecond
		span := Span{
			TraceID:      firstString(rawSpan.TraceID, raw.TraceID),
			SpanID:       rawSpan.SpanID,
			ParentSpanID: parentID,
			Service:      service,
			Operation:    rawSpan.OperationName,
			StartTime:    start,
			Duration:     duration,
			Status:       jaegerStatus(attributes),
			Attributes:   attributes,
		}
		trace.Spans = append(trace.Spans, span)
		end := start.Add(duration)
		if trace.StartTime.IsZero() || start.Before(trace.StartTime) {
			trace.StartTime = start
		}
		if end.After(traceEnd) {
			traceEnd = end
		}
		if parentID == "" && trace.RootOperation == "" {
			trace.RootService = service
			trace.RootOperation = rawSpan.OperationName
		}
	}
	sort.SliceStable(trace.Spans, func(i, j int) bool {
		return trace.Spans[i].StartTime.Before(trace.Spans[j].StartTime)
	})
	if !trace.StartTime.IsZero() && !traceEnd.IsZero() {
		trace.Duration = traceEnd.Sub(trace.StartTime)
	}
	return trace
}

func tagsToAttributes(tags []jaegerTag) map[string]string {
	attributes := make(map[string]string, len(tags))
	for _, tag := range tags {
		attributes[tag.Key] = fmt.Sprint(tag.Value)
	}
	return attributes
}

func jaegerStatus(attributes map[string]string) string {
	if strings.EqualFold(attributes["error"], "true") {
		return "ERROR"
	}
	for _, key := range []string{"otel.status_code", "status.code"} {
		switch strings.ToUpper(attributes[key]) {
		case "ERROR":
			return "ERROR"
		case "OK":
			return "OK"
		}
	}
	return ""
}

func firstString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
