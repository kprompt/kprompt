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

type tempoSearchResponse struct {
	Traces []tempoTraceSummary `json:"traces"`
}

type tempoTraceSummary struct {
	TraceID           string          `json:"traceID"`
	RootServiceName   string          `json:"rootServiceName"`
	RootTraceName     string          `json:"rootTraceName"`
	StartTimeUnixNano json.RawMessage `json:"startTimeUnixNano"`
	DurationMs        json.RawMessage `json:"durationMs"`
}

type tempoTraceResponse struct {
	Batches       []tempoResourceSpans `json:"batches"`
	ResourceSpans []tempoResourceSpans `json:"resourceSpans"`
}

type tempoResourceSpans struct {
	Resource                    tempoResource     `json:"resource"`
	ScopeSpans                  []tempoScopeSpans `json:"scopeSpans"`
	InstrumentationLibrarySpans []tempoScopeSpans `json:"instrumentationLibrarySpans"`
}

type tempoResource struct {
	Attributes []tempoAttribute `json:"attributes"`
}

type tempoScopeSpans struct {
	Spans []tempoSpan `json:"spans"`
}

type tempoSpan struct {
	TraceID           string           `json:"traceId"`
	SpanID            string           `json:"spanId"`
	ParentSpanID      string           `json:"parentSpanId"`
	Name              string           `json:"name"`
	StartTimeUnixNano json.RawMessage  `json:"startTimeUnixNano"`
	EndTimeUnixNano   json.RawMessage  `json:"endTimeUnixNano"`
	Attributes        []tempoAttribute `json:"attributes"`
	Status            tempoStatus      `json:"status"`
}

type tempoStatus struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

type tempoAttribute struct {
	Key   string        `json:"key"`
	Value tempoAnyValue `json:"value"`
}

type tempoAnyValue struct {
	StringValue string          `json:"stringValue"`
	BoolValue   *bool           `json:"boolValue"`
	IntValue    json.RawMessage `json:"intValue"`
	DoubleValue *float64        `json:"doubleValue"`
}

func (c *Client) searchTempo(ctx context.Context, req SearchRequest) ([]Trace, error) {
	query := url.Values{
		"q":     {tempoTraceQL(req.Service, req.Operation)},
		"start": {strconv.FormatInt(req.Start.Unix(), 10)},
		"end":   {strconv.FormatInt(req.End.Unix(), 10)},
		"limit": {strconv.Itoa(req.Limit)},
	}
	body, err := c.get(ctx, "/api/search", query)
	if err != nil {
		return nil, err
	}
	var response tempoSearchResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("decode Tempo search response: %w", err)
	}
	traces := make([]Trace, 0, len(response.Traces))
	for _, summary := range response.Traces {
		if strings.TrimSpace(summary.TraceID) == "" {
			continue
		}
		trace, err := c.GetTrace(ctx, summary.TraceID)
		if err != nil {
			return nil, fmt.Errorf("fetch Tempo trace %s: %w", summary.TraceID, err)
		}
		applyTempoSummary(&trace, summary)
		traces = append(traces, trace)
	}
	return traces, nil
}

func decodeTempoTrace(traceID string, body []byte) (Trace, error) {
	var response tempoTraceResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return Trace{}, fmt.Errorf("decode Tempo trace response: %w", err)
	}
	batches := response.ResourceSpans
	if len(batches) == 0 {
		batches = response.Batches
	}
	if batches == nil {
		return Trace{}, fmt.Errorf("decode Tempo trace response: missing resource spans")
	}
	trace := Trace{TraceID: traceID}
	var traceEnd time.Time
	for _, batch := range batches {
		resourceAttributes := tempoAttributes(batch.Resource.Attributes)
		service := resourceAttributes["service.name"]
		scopes := append([]tempoScopeSpans(nil), batch.ScopeSpans...)
		scopes = append(scopes, batch.InstrumentationLibrarySpans...)
		for _, scope := range scopes {
			for _, rawSpan := range scope.Spans {
				start, err := unixNanoTime(rawSpan.StartTimeUnixNano)
				if err != nil {
					return Trace{}, fmt.Errorf("decode Tempo span start: %w", err)
				}
				end, err := unixNanoTime(rawSpan.EndTimeUnixNano)
				if err != nil {
					return Trace{}, fmt.Errorf("decode Tempo span end: %w", err)
				}
				attributes := tempoAttributes(rawSpan.Attributes)
				for key, value := range resourceAttributes {
					if _, exists := attributes[key]; !exists {
						attributes[key] = value
					}
				}
				span := Span{
					TraceID:      firstString(rawSpan.TraceID, traceID),
					SpanID:       rawSpan.SpanID,
					ParentSpanID: rawSpan.ParentSpanID,
					Service:      service,
					Operation:    rawSpan.Name,
					StartTime:    start,
					Duration:     end.Sub(start),
					Status:       tempoStatusLabel(rawSpan.Status),
					Attributes:   attributes,
				}
				trace.Spans = append(trace.Spans, span)
				if trace.StartTime.IsZero() || start.Before(trace.StartTime) {
					trace.StartTime = start
				}
				if end.After(traceEnd) {
					traceEnd = end
				}
				if rawSpan.ParentSpanID == "" && trace.RootOperation == "" {
					trace.RootService = service
					trace.RootOperation = rawSpan.Name
				}
			}
		}
	}
	sort.SliceStable(trace.Spans, func(i, j int) bool {
		return trace.Spans[i].StartTime.Before(trace.Spans[j].StartTime)
	})
	if !trace.StartTime.IsZero() && !traceEnd.IsZero() {
		trace.Duration = traceEnd.Sub(trace.StartTime)
	}
	return trace, nil
}

func tempoTraceQL(service, operation string) string {
	clauses := []string{
		fmt.Sprintf(`resource.service.name = "%s"`, escapeTraceQL(service)),
	}
	if operation != "" {
		clauses = append(
			clauses,
			fmt.Sprintf(`name = "%s"`, escapeTraceQL(operation)),
		)
	}
	return "{ " + strings.Join(clauses, " && ") + " }"
}

func escapeTraceQL(value string) string {
	value = strings.ReplaceAll(value, `\`, `\\`)
	return strings.ReplaceAll(value, `"`, `\"`)
}

func applyTempoSummary(trace *Trace, summary tempoTraceSummary) {
	if trace.TraceID == "" {
		trace.TraceID = summary.TraceID
	}
	if trace.RootService == "" {
		trace.RootService = summary.RootServiceName
	}
	if trace.RootOperation == "" {
		trace.RootOperation = summary.RootTraceName
	}
	if trace.StartTime.IsZero() {
		if start, err := unixNanoTime(summary.StartTimeUnixNano); err == nil {
			trace.StartTime = start
		}
	}
	if trace.Duration == 0 {
		if duration, err := rawFloat(summary.DurationMs); err == nil {
			trace.Duration = time.Duration(duration * float64(time.Millisecond))
		}
	}
}

func tempoAttributes(raw []tempoAttribute) map[string]string {
	attributes := make(map[string]string, len(raw))
	for _, attribute := range raw {
		attributes[attribute.Key] = attribute.Value.String()
	}
	return attributes
}

func (value tempoAnyValue) String() string {
	switch {
	case value.StringValue != "":
		return value.StringValue
	case value.BoolValue != nil:
		return strconv.FormatBool(*value.BoolValue)
	case len(value.IntValue) > 0:
		return strings.Trim(string(value.IntValue), `"`)
	case value.DoubleValue != nil:
		return strconv.FormatFloat(*value.DoubleValue, 'g', -1, 64)
	default:
		return ""
	}
}

func tempoStatusLabel(status tempoStatus) string {
	raw := strings.Trim(strings.TrimSpace(string(status.Code)), `"`)
	switch strings.ToUpper(raw) {
	case "2", "STATUS_CODE_ERROR", "ERROR":
		return "ERROR"
	case "1", "STATUS_CODE_OK", "OK":
		return "OK"
	default:
		return ""
	}
}

func unixNanoTime(raw json.RawMessage) (time.Time, error) {
	value, err := rawInt64(raw)
	if err != nil {
		return time.Time{}, err
	}
	if value == 0 {
		return time.Time{}, nil
	}
	return time.Unix(0, value).UTC(), nil
}

func rawInt64(raw json.RawMessage) (int64, error) {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return 0, nil
	}
	return strconv.ParseInt(text, 10, 64)
}

func rawFloat(raw json.RawMessage) (float64, error) {
	text := strings.Trim(strings.TrimSpace(string(raw)), `"`)
	if text == "" || text == "null" {
		return 0, nil
	}
	return strconv.ParseFloat(text, 64)
}
