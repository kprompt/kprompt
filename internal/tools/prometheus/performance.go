package prometheus

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Querier is the subset of Client used by performance diagnosis.
type Querier interface {
	Query(ctx context.Context, promQL string, at time.Time) (Result, error)
}

// PerformanceRequest identifies a Kubernetes workload and metrics window.
type PerformanceRequest struct {
	Workload  string
	Namespace string
	Window    time.Duration
}

// Measurement is one Prometheus signal used in a performance report.
type Measurement struct {
	Name     string   `json:"name"`
	Value    *float64 `json:"value,omitempty"`
	Unit     string   `json:"unit,omitempty"`
	Query    string   `json:"query"`
	Error    string   `json:"error,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// ScaleSuggestion is a non-mutating recommendation derived from metrics.
type ScaleSuggestion struct {
	Current   int32  `json:"current"`
	Suggested int32  `json:"suggested"`
	Reason    string `json:"reason"`
}

// PerformanceReport summarizes CPU, memory, latency, replicas, and HPA signals.
type PerformanceReport struct {
	Workload   string           `json:"workload"`
	Namespace  string           `json:"namespace"`
	Window     string           `json:"window"`
	Summary    string           `json:"summary"`
	Metrics    []Measurement    `json:"metrics"`
	Findings   []string         `json:"findings"`
	Suggestion *ScaleSuggestion `json:"suggestion,omitempty"`
}

type metricSpec struct {
	key   string
	name  string
	unit  string
	query string
}

type metricOutcome struct {
	index       int
	measurement Measurement
}

// ExplainPerformance queries Prometheus signals and produces a human-oriented report.
func ExplainPerformance(
	ctx context.Context,
	querier Querier,
	req PerformanceRequest,
) (PerformanceReport, error) {
	if querier == nil {
		return PerformanceReport{}, fmt.Errorf("Prometheus querier is required")
	}
	req.Workload = strings.TrimSpace(req.Workload)
	if req.Workload == "" {
		return PerformanceReport{}, fmt.Errorf("performance workload is required")
	}
	req.Namespace = strings.TrimSpace(req.Namespace)
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Window <= 0 {
		req.Window = 15 * time.Minute
	}
	if req.Window < time.Minute || req.Window > 24*time.Hour {
		return PerformanceReport{}, fmt.Errorf("performance window must be between 1m and 24h")
	}

	specs := performanceQueries(req)
	outcomes := make(chan metricOutcome, len(specs))
	var wait sync.WaitGroup
	for index, spec := range specs {
		wait.Add(1)
		go func(index int, spec metricSpec) {
			defer wait.Done()
			measurement := Measurement{
				Name:  spec.name,
				Unit:  spec.unit,
				Query: spec.query,
			}
			result, err := querier.Query(ctx, spec.query, time.Time{})
			if err != nil {
				measurement.Error = err.Error()
			} else {
				measurement.Warnings = append([]string(nil), result.Warnings...)
				if value, ok, valueErr := FirstValue(result); valueErr != nil {
					measurement.Error = valueErr.Error()
				} else if ok {
					measurement.Value = &value
				}
			}
			outcomes <- metricOutcome{index: index, measurement: measurement}
		}(index, spec)
	}
	wait.Wait()
	close(outcomes)

	report := PerformanceReport{
		Workload:  req.Workload,
		Namespace: req.Namespace,
		Window:    formatPromDuration(req.Window),
		Metrics:   make([]Measurement, len(specs)),
	}
	failed := 0
	var firstError string
	for outcome := range outcomes {
		report.Metrics[outcome.index] = outcome.measurement
		if outcome.measurement.Error != "" {
			failed++
			if firstError == "" {
				firstError = outcome.measurement.Error
			}
		}
	}
	if failed == len(specs) {
		return report, fmt.Errorf("all Prometheus performance queries failed: %s", firstError)
	}
	analyzePerformance(&report, specs)
	return report, nil
}

// FirstValue returns the first scalar or series sample from a Prometheus result.
func FirstValue(result Result) (float64, bool, error) {
	var raw string
	switch {
	case result.Scalar != nil:
		raw = result.Scalar.Value
	case len(result.Series) > 0 && len(result.Series[0].Samples) > 0:
		raw = result.Series[0].Samples[0].Value
	default:
		return 0, false, nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0, false, fmt.Errorf("parse Prometheus value %q: %w", raw, err)
	}
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, false, nil
	}
	return value, true, nil
}

func performanceQueries(req PerformanceRequest) []metricSpec {
	namespace := strconv.Quote(req.Namespace)
	workload := strconv.Quote(req.Workload)
	podPattern := strconv.Quote("^" + regexp.QuoteMeta(req.Workload) + "-.*")
	window := formatPromDuration(req.Window)
	podSelector := fmt.Sprintf(
		`namespace=%s,pod=~%s`,
		namespace,
		podPattern,
	)
	hpaPattern := strconv.Quote("^" + regexp.QuoteMeta(req.Workload) + ".*")

	return []metricSpec{
		{
			key:  "cpu_usage",
			name: "CPU usage",
			unit: "cores",
			query: fmt.Sprintf(
				`sum(rate(container_cpu_usage_seconds_total{%s,container!="",container!="POD"}[%s]))`,
				podSelector,
				window,
			),
		},
		{
			key:  "cpu_request",
			name: "CPU request",
			unit: "cores",
			query: fmt.Sprintf(
				`sum(kube_pod_container_resource_requests{%s,resource="cpu",unit="core"})`,
				podSelector,
			),
		},
		{
			key:  "memory_usage",
			name: "Memory working set",
			unit: "bytes",
			query: fmt.Sprintf(
				`sum(container_memory_working_set_bytes{%s,container!="",container!="POD"})`,
				podSelector,
			),
		},
		{
			key:  "memory_request",
			name: "Memory request",
			unit: "bytes",
			query: fmt.Sprintf(
				`sum(kube_pod_container_resource_requests{%s,resource="memory",unit="byte"})`,
				podSelector,
			),
		},
		{
			key:  "p95_latency",
			name: "HTTP p95 latency",
			unit: "seconds",
			query: fmt.Sprintf(
				`histogram_quantile(0.95, sum by (le) (`+
					`rate(http_request_duration_seconds_bucket{%s}[%s]) or `+
					`rate(http_server_request_duration_seconds_bucket{%s}[%s])))`,
				podSelector,
				window,
				podSelector,
				window,
			),
		},
		{
			key:  "replicas",
			name: "Deployment replicas",
			unit: "replicas",
			query: fmt.Sprintf(
				`kube_deployment_status_replicas{namespace=%s,deployment=%s}`,
				namespace,
				workload,
			),
		},
		{
			key:  "hpa_current",
			name: "HPA current replicas",
			unit: "replicas",
			query: fmt.Sprintf(
				`kube_horizontalpodautoscaler_status_current_replicas{namespace=%s,horizontalpodautoscaler=~%s}`,
				namespace,
				hpaPattern,
			),
		},
		{
			key:  "hpa_desired",
			name: "HPA desired replicas",
			unit: "replicas",
			query: fmt.Sprintf(
				`kube_horizontalpodautoscaler_status_desired_replicas{namespace=%s,horizontalpodautoscaler=~%s}`,
				namespace,
				hpaPattern,
			),
		},
		{
			key:  "hpa_max",
			name: "HPA max replicas",
			unit: "replicas",
			query: fmt.Sprintf(
				`kube_horizontalpodautoscaler_spec_max_replicas{namespace=%s,horizontalpodautoscaler=~%s}`,
				namespace,
				hpaPattern,
			),
		},
	}
}

func analyzePerformance(report *PerformanceReport, specs []metricSpec) {
	values := make(map[string]float64, len(specs))
	available := make(map[string]bool, len(specs))
	for index, metric := range report.Metrics {
		if metric.Value == nil {
			continue
		}
		values[specs[index].key] = *metric.Value
		available[specs[index].key] = true
	}

	var cpuRatio, memoryRatio float64
	if available["cpu_usage"] && available["cpu_request"] && values["cpu_request"] > 0 {
		cpuRatio = values["cpu_usage"] / values["cpu_request"]
		report.Findings = append(
			report.Findings,
			fmt.Sprintf("CPU uses %.0f%% of requested capacity.", cpuRatio*100),
		)
	}
	if available["memory_usage"] && available["memory_request"] && values["memory_request"] > 0 {
		memoryRatio = values["memory_usage"] / values["memory_request"]
		report.Findings = append(
			report.Findings,
			fmt.Sprintf("Memory uses %.0f%% of requested capacity.", memoryRatio*100),
		)
	}
	if available["p95_latency"] {
		report.Findings = append(
			report.Findings,
			fmt.Sprintf("HTTP p95 latency is %.3fs.", values["p95_latency"]),
		)
	}

	hpaCurrent, hasHPACurrent := values["hpa_current"]
	hpaDesired, hasHPADesired := values["hpa_desired"]
	if hasHPACurrent && hasHPADesired {
		report.Findings = append(
			report.Findings,
			fmt.Sprintf("HPA reports %.0f current and %.0f desired replicas.", hpaCurrent, hpaDesired),
		)
		if hpaDesired > hpaCurrent {
			report.Suggestion = &ScaleSuggestion{
				Current:   int32(math.Round(hpaCurrent)),
				Suggested: int32(math.Round(hpaDesired)),
				Reason:    "HPA desired replicas exceed current replicas",
			}
		}
	}

	latency := values["p95_latency"]
	switch {
	case available["p95_latency"] && latency >= 1:
		report.Summary = "High p95 latency is the strongest signal."
	case memoryRatio >= 0.9:
		report.Summary = "Memory pressure is the strongest signal."
	case cpuRatio >= 0.85:
		report.Summary = "CPU saturation is the strongest signal."
	case len(report.Findings) > 0:
		report.Summary = "No severe performance signal crossed the built-in thresholds."
	default:
		report.Summary = "Prometheus returned insufficient matching series for a diagnosis."
	}

	if report.Suggestion == nil && (cpuRatio >= 0.85 || memoryRatio >= 0.9) {
		if replicas, ok := values["replicas"]; ok {
			current := int32(math.Round(replicas))
			if current < 1 {
				current = 1
			}
			report.Suggestion = &ScaleSuggestion{
				Current:   current,
				Suggested: current + 1,
				Reason:    "resource utilization crossed the diagnostic threshold",
			}
		}
	}
}

func formatPromDuration(value time.Duration) string {
	if value%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(value/time.Hour))
	}
	if value%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(value/time.Minute))
	}
	return fmt.Sprintf("%ds", int64(value/time.Second))
}
