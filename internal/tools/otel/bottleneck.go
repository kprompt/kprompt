package otel

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"time"
)

const (
	maxBottlenecks         = 5
	minBottleneckShare     = 0.10
	minBottleneckDuration  = 25 * time.Millisecond
	minSingleSpanHighlight = 100 * time.Millisecond
)

// Bottleneck is a slow or failing span called out in human narration.
type Bottleneck struct {
	SpanID    string        `json:"spanId"`
	Service   string        `json:"service,omitempty"`
	Operation string        `json:"operation"`
	Duration  time.Duration `json:"duration"`
	Share     float64       `json:"share"`
	Status    string        `json:"status,omitempty"`
	Message   string        `json:"message"`
}

// TraceReport is a walkable trace plus bottleneck narration.
type TraceReport struct {
	Trace       Trace        `json:"-"`
	Spans       []SpanRow    `json:"spans"`
	Summary     string       `json:"summary"`
	Bottlenecks []Bottleneck `json:"bottlenecks"`
}

type spanExclusive struct {
	span      Span
	exclusive time.Duration
}

// AnalyzeTrace walks spans and narrates the dominant wait points.
func AnalyzeTrace(trace Trace) TraceReport {
	report := TraceReport{
		Trace: trace,
		Spans: WalkSpans(trace),
	}
	total := trace.Duration
	if total <= 0 {
		for _, span := range trace.Spans {
			if span.Duration > total {
				total = span.Duration
			}
		}
	}
	exclusives := exclusiveSpans(trace.Spans)
	sort.SliceStable(exclusives, func(i, j int) bool {
		if exclusives[i].exclusive == exclusives[j].exclusive {
			return exclusives[i].span.StartTime.Before(exclusives[j].span.StartTime)
		}
		return exclusives[i].exclusive > exclusives[j].exclusive
	})

	threshold := minBottleneckDuration
	if total > 0 {
		shareFloor := time.Duration(float64(total) * minBottleneckShare)
		if shareFloor > threshold {
			threshold = shareFloor
		}
	}

	seen := map[string]bool{}
	for _, item := range exclusives {
		if len(report.Bottlenecks) >= maxBottlenecks {
			break
		}
		span := item.span
		if span.SpanID != "" && seen[span.SpanID] {
			continue
		}
		isError := strings.EqualFold(span.Status, "ERROR")
		if !isError && item.exclusive < threshold {
			continue
		}
		if !isError && len(trace.Spans) == 1 && item.exclusive < minSingleSpanHighlight {
			continue
		}
		share := 0.0
		if total > 0 {
			share = float64(item.exclusive) / float64(total)
		}
		finding := Bottleneck{
			SpanID:    span.SpanID,
			Service:   span.Service,
			Operation: span.Operation,
			Duration:  item.exclusive,
			Share:     roundShare(share),
			Status:    span.Status,
			Message:   bottleneckMessage(span, item.exclusive, share, isError),
		}
		report.Bottlenecks = append(report.Bottlenecks, finding)
		if span.SpanID != "" {
			seen[span.SpanID] = true
		}
	}

	report.Summary = bottleneckSummary(trace, report.Bottlenecks)
	return report
}

func exclusiveSpans(spans []Span) []spanExclusive {
	children := make(map[string][]Span, len(spans))
	known := make(map[string]bool, len(spans))
	for _, span := range spans {
		known[span.SpanID] = true
	}
	for _, span := range spans {
		if span.ParentSpanID == "" || !known[span.ParentSpanID] {
			continue
		}
		children[span.ParentSpanID] = append(children[span.ParentSpanID], span)
	}

	out := make([]spanExclusive, 0, len(spans))
	for _, span := range spans {
		childSum := time.Duration(0)
		for _, child := range children[span.SpanID] {
			childSum += child.Duration
		}
		exclusive := span.Duration - childSum
		if exclusive < 0 {
			exclusive = 0
		}
		if exclusive == 0 && len(children[span.SpanID]) == 0 {
			exclusive = span.Duration
		}
		out = append(out, spanExclusive{span: span, exclusive: exclusive})
	}
	return out
}

func bottleneckMessage(span Span, exclusive time.Duration, share float64, isError bool) string {
	target := strings.TrimSpace(span.Service)
	if target == "" {
		target = strings.TrimSpace(span.Operation)
	}
	if target == "" {
		target = "span"
	}
	wait := formatWait(exclusive)
	switch {
	case isError && span.Service != "":
		return fmt.Sprintf("error in %s after %s (%s)", span.Service, wait, firstString(span.Operation, "operation"))
	case isError:
		return fmt.Sprintf("error after %s on %s", wait, target)
	case span.Service != "":
		msg := fmt.Sprintf("waited %s for %s", wait, span.Service)
		if op := strings.TrimSpace(span.Operation); op != "" {
			msg += fmt.Sprintf(" (%s)", op)
		}
		if share >= minBottleneckShare {
			msg += fmt.Sprintf(" — %.0f%% of trace", share*100)
		}
		return msg
	default:
		msg := fmt.Sprintf("waited %s on %s", wait, target)
		if share >= minBottleneckShare {
			msg += fmt.Sprintf(" — %.0f%% of trace", share*100)
		}
		return msg
	}
}

func bottleneckSummary(trace Trace, bottlenecks []Bottleneck) string {
	root := strings.TrimSpace(trace.RootService)
	if root != "" && strings.TrimSpace(trace.RootOperation) != "" {
		root += " — " + strings.TrimSpace(trace.RootOperation)
	} else if root == "" {
		root = strings.TrimSpace(trace.RootOperation)
	}
	prefix := "Trace"
	if root != "" {
		prefix = root
	}
	if len(bottlenecks) == 0 {
		return fmt.Sprintf("%s completed in %s with no significant span bottlenecks.", prefix, formatWait(trace.Duration))
	}
	return fmt.Sprintf("Main bottleneck: %s.", bottlenecks[0].Message)
}

func formatWait(d time.Duration) string {
	if d <= 0 {
		return "0s"
	}
	if d >= time.Second {
		seconds := float64(d) / float64(time.Second)
		if math.Abs(seconds-math.Round(seconds)) < 0.05 {
			return fmt.Sprintf("%.0fs", seconds)
		}
		return fmt.Sprintf("%.1fs", seconds)
	}
	if d >= time.Millisecond {
		return fmt.Sprintf("%dms", d/time.Millisecond)
	}
	return d.String()
}

func roundShare(share float64) float64 {
	if share <= 0 {
		return 0
	}
	return math.Round(share*1000) / 1000
}
