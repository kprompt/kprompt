package optimize

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

const (
	// Idle when average CPU usage is below this fraction of request.
	idleCPURatio = 0.10
	// Idle when average memory usage is below this fraction of request.
	idleMemoryRatio = 0.20
	// Cap Prometheus fan-out for large inventories.
	maxIdleScan      = 40
	idleQueryWorkers = 6
)

// IdleWorkload is one underutilized workload signal (T-054).
type IdleWorkload struct {
	Kind                 string   `json:"kind"`
	Namespace            string   `json:"namespace"`
	Name                 string   `json:"name"`
	CPUOfRequestPct      *float64 `json:"cpuOfRequestPct,omitempty"`
	MemoryOfRequestPct   *float64 `json:"memoryOfRequestPct,omitempty"`
	Idle                 bool     `json:"idle"`
	Message              string   `json:"message,omitempty"`
}

// IdleResult is Prometheus-backed idle detection over inventoried workloads.
type IdleResult struct {
	Workloads []IdleWorkload
	IdleCount int
	Scanned   int
	Warnings  []string
	Skipped   bool
	SkipReason string
}

// DetectIdle compares Prometheus usage to requests for inventoried workloads.
// Partial query failures become warnings; missing Prometheus skips the section.
func DetectIdle(
	ctx context.Context,
	querier toolprometheus.Querier,
	workloads []Workload,
	window time.Duration,
) IdleResult {
	if querier == nil {
		return IdleResult{
			Skipped:    true,
			SkipReason: "Prometheus is not configured; idle detection skipped",
		}
	}
	if window <= 0 {
		window = time.Hour
	}
	if len(workloads) == 0 {
		return IdleResult{
			Workloads: []IdleWorkload{},
			Scanned:   0,
		}
	}

	scan := workloads
	var warnings []string
	if len(scan) > maxIdleScan {
		warnings = append(warnings, fmt.Sprintf(
			"idle scan limited to first %d of %d workloads",
			maxIdleScan, len(scan),
		))
		scan = scan[:maxIdleScan]
	}

	type job struct {
		index int
		wl    Workload
	}
	jobs := make(chan job)
	results := make([]IdleWorkload, len(scan))
	var warnMu sync.Mutex
	var wait sync.WaitGroup

	workers := idleQueryWorkers
	if len(scan) < workers {
		workers = len(scan)
	}
	for w := 0; w < workers; w++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := range jobs {
				sig, warns := probeIdleWorkload(ctx, querier, j.wl, window)
				results[j.index] = sig
				if len(warns) > 0 {
					warnMu.Lock()
					warnings = append(warnings, warns...)
					warnMu.Unlock()
				}
			}
		}()
	}
	for i, wl := range scan {
		select {
		case <-ctx.Done():
			close(jobs)
			wait.Wait()
			return IdleResult{
				Workloads: nil,
				Scanned:   0,
				Warnings:  append(warnings, ctx.Err().Error()),
				Skipped:   true,
				SkipReason: "idle detection canceled",
			}
		case jobs <- job{index: i, wl: wl}:
		}
	}
	close(jobs)
	wait.Wait()

	out := IdleResult{
		Workloads: make([]IdleWorkload, 0, len(results)),
		Scanned:   len(scan),
		Warnings:  warnings,
	}
	for _, sig := range results {
		if sig.Idle {
			out.IdleCount++
			out.Workloads = append(out.Workloads, sig)
		}
	}
	return out
}

// ApplyIdle merges idle findings into the optimize report (read-only).
func ApplyIdle(rep *Report, idle IdleResult) {
	if rep == nil {
		return
	}
	if len(idle.Workloads) > 0 {
		rep.Idle = idle.Workloads
	}
	if idle.Skipped {
		msg := idle.SkipReason
		if msg == "" {
			msg = "Idle detection skipped"
		}
		rep.Sections.Idle = SectionStatus{Status: SectionSkipped, Message: msg}
		rep.Findings = append(rep.Findings, Finding{
			Code:     "optimize.idle.skipped",
			Severity: "medium",
			Title:    "Idle detection skipped",
			Message:  msg,
		})
		for _, w := range idle.Warnings {
			rep.Findings = append(rep.Findings, Finding{
				Code:     "optimize.idle.warning",
				Severity: "medium",
				Title:    "Prometheus warning",
				Message:  w,
			})
		}
		return
	}

	rep.Sections.Idle = SectionStatus{
		Status: SectionReady,
		Message: fmt.Sprintf(
			"%d idle/underutilized of %d scanned workloads",
			idle.IdleCount, idle.Scanned,
		),
	}

	rep.Findings = append(rep.Findings, Finding{
		Code:     "optimize.idle.summary",
		Severity: SeverityInfo,
		Title:    "Idle / underutilized",
		Message: fmt.Sprintf(
			"%d of %d scanned workloads averaged below idle thresholds over %s",
			idle.IdleCount, idle.Scanned, rep.Window,
		),
	})

	for _, w := range idle.Workloads {
		severity := "low"
		if w.CPUOfRequestPct != nil && *w.CPUOfRequestPct < 5 {
			severity = "medium"
		}
		rep.Findings = append(rep.Findings, Finding{
			Code:      "optimize.idle.workload",
			Severity:  severity,
			Title:     "Underutilized workload",
			Message:   w.Message,
			Resource:  fmt.Sprintf("%s/%s", w.Kind, w.Name),
			Namespace: w.Namespace,
		})
	}
	for _, w := range idle.Warnings {
		rep.Findings = append(rep.Findings, Finding{
			Code:     "optimize.idle.warning",
			Severity: "medium",
			Title:    "Prometheus warning",
			Message:  w,
		})
	}

	if idle.IdleCount > 0 {
		rep.Suggestions = append(rep.Suggestions, Suggestion{
			Code:       "optimize.idle.review",
			Title:      "Review idle workloads",
			Message:    fmt.Sprintf("%d workloads look underutilized versus requests; confirm before scaling or rightsizing (no auto-apply).", idle.IdleCount),
			ActionHint: "none",
		})
	}

	// Refresh summary to mention idle without wiping inventory context.
	if strings.Contains(rep.Summary, "Idle, rightsizing") {
		rep.Summary = strings.Replace(
			rep.Summary,
			"Idle, rightsizing, and HPA sections remain pending — no mutations.",
			fmt.Sprintf("%d idle/underutilized findings; rightsizing and HPA remain pending — no mutations.", idle.IdleCount),
			1,
		)
	} else if idle.IdleCount > 0 {
		rep.Summary += fmt.Sprintf(" Found %d idle/underutilized workloads.", idle.IdleCount)
	}
}

func probeIdleWorkload(
	ctx context.Context,
	querier toolprometheus.Querier,
	wl Workload,
	window time.Duration,
) (IdleWorkload, []string) {
	sig := IdleWorkload{
		Kind:      wl.Kind,
		Namespace: wl.Namespace,
		Name:      wl.Name,
	}
	ns := strings.TrimSpace(wl.Namespace)
	if ns == "" {
		ns = "default"
	}
	namespace := strconv.Quote(ns)
	podPattern := strconv.Quote("^" + regexp.QuoteMeta(wl.Name) + "-.*")
	podSelector := fmt.Sprintf(`namespace=%s,pod=~%s`, namespace, podPattern)
	win := formatPromWindow(window)

	queries := []struct {
		key   string
		query string
	}{
		{"cpu_usage", fmt.Sprintf(
			`sum(rate(container_cpu_usage_seconds_total{%s,container!="",container!="POD"}[%s]))`,
			podSelector, win,
		)},
		{"cpu_request", fmt.Sprintf(
			`sum(kube_pod_container_resource_requests{%s,resource="cpu",unit="core"})`,
			podSelector,
		)},
		{"memory_usage", fmt.Sprintf(
			`sum(container_memory_working_set_bytes{%s,container!="",container!="POD"})`,
			podSelector,
		)},
		{"memory_request", fmt.Sprintf(
			`sum(kube_pod_container_resource_requests{%s,resource="memory",unit="byte"})`,
			podSelector,
		)},
	}

	values := map[string]float64{}
	var warnings []string
	for _, q := range queries {
		result, err := querier.Query(ctx, q.query, time.Time{})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s/%s %s: %v", wl.Kind, wl.Name, q.key, err))
			continue
		}
		for _, w := range result.Warnings {
			warnings = append(warnings, fmt.Sprintf("%s/%s %s: %s", wl.Kind, wl.Name, q.key, w))
		}
		value, ok, valueErr := toolprometheus.FirstValue(result)
		if valueErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s/%s %s: %v", wl.Kind, wl.Name, q.key, valueErr))
			continue
		}
		if ok {
			values[q.key] = value
		}
	}

	var parts []string
	if cpuReq, ok := values["cpu_request"]; ok && cpuReq > 0 {
		if cpuUse, ok := values["cpu_usage"]; ok {
			pct := (cpuUse / cpuReq) * 100
			if math.IsNaN(pct) || math.IsInf(pct, 0) {
				pct = 0
			}
			sig.CPUOfRequestPct = &pct
			if cpuUse/cpuReq < idleCPURatio {
				sig.Idle = true
				parts = append(parts, fmt.Sprintf("averaged %.0f%% CPU of request over %s", pct, formatWindow(window)))
			}
		}
	}
	if memReq, ok := values["memory_request"]; ok && memReq > 0 {
		if memUse, ok := values["memory_usage"]; ok {
			pct := (memUse / memReq) * 100
			if math.IsNaN(pct) || math.IsInf(pct, 0) {
				pct = 0
			}
			sig.MemoryOfRequestPct = &pct
			if memUse/memReq < idleMemoryRatio {
				sig.Idle = true
				parts = append(parts, fmt.Sprintf("averaged %.0f%% memory of request over %s", pct, formatWindow(window)))
			}
		}
	}

	if sig.Idle {
		sig.Message = fmt.Sprintf("%s/%s %s", wl.Kind, wl.Name, strings.Join(parts, "; "))
	}
	return sig, warnings
}

func formatPromWindow(d time.Duration) string {
	if d%time.Hour == 0 {
		return fmt.Sprintf("%dh", int64(d/time.Hour))
	}
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dm", int64(d/time.Minute))
	}
	return fmt.Sprintf("%ds", int64(d/time.Second))
}
