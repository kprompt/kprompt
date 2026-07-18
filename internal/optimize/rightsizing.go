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

	"k8s.io/apimachinery/pkg/api/resource"

	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

const (
	// Target CPU utilization when deriving a request from average usage.
	rightsizeCPUTarget = 0.70
	// Headroom applied on memory p95 when deriving a request.
	rightsizeMemHeadroom = 1.25
	// Limit suggestion multiplier over suggested request.
	rightsizeLimitMult = 1.5
	// Only narrate when suggested differs from current by at least this fraction.
	rightsizeMinDelta = 0.25
	maxRightsizingScan      = 40
	rightsizingQueryWorkers = 6
)

// RightsizingDelta is one concrete request/limit change suggestion (T-055).
type RightsizingDelta struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Resource  string `json:"resource"` // cpu | memory
	Field     string `json:"field"`    // request | limit
	Current   string `json:"current"`
	Suggested string `json:"suggested"`
	Direction string `json:"direction"` // lower | raise
	Message   string `json:"message"`
}

// RightsizingResult holds rightsizing suggestions from usage vs inventory.
type RightsizingResult struct {
	Deltas     []RightsizingDelta
	Scanned    int
	Warnings   []string
	Skipped    bool
	SkipReason string
}

// SuggestRightsizing proposes request/limit deltas from Prometheus usage percentiles.
// Suggestions only — never mutates the cluster. Partial Prom failures → warnings.
func SuggestRightsizing(
	ctx context.Context,
	querier toolprometheus.Querier,
	workloads []Workload,
	window time.Duration,
) RightsizingResult {
	if querier == nil {
		return RightsizingResult{
			Skipped:    true,
			SkipReason: "Prometheus is not configured; rightsizing skipped",
		}
	}
	if window <= 0 {
		window = time.Hour
	}
	if len(workloads) == 0 {
		return RightsizingResult{Deltas: []RightsizingDelta{}, Scanned: 0}
	}

	scan := workloads
	var warnings []string
	if len(scan) > maxRightsizingScan {
		warnings = append(warnings, fmt.Sprintf(
			"rightsizing scan limited to first %d of %d workloads",
			maxRightsizingScan, len(scan),
		))
		scan = scan[:maxRightsizingScan]
	}

	type job struct {
		index int
		wl    Workload
	}
	jobs := make(chan job)
	type outcome struct {
		deltas   []RightsizingDelta
		warnings []string
	}
	outcomes := make([]outcome, len(scan))
	var wait sync.WaitGroup

	workers := rightsizingQueryWorkers
	if len(scan) < workers {
		workers = len(scan)
	}
	for w := 0; w < workers; w++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := range jobs {
				deltas, warns := probeRightsizing(ctx, querier, j.wl, window)
				outcomes[j.index] = outcome{deltas: deltas, warnings: warns}
			}
		}()
	}
	for i, wl := range scan {
		select {
		case <-ctx.Done():
			close(jobs)
			wait.Wait()
			return RightsizingResult{
				Skipped:    true,
				SkipReason: "rightsizing canceled",
				Warnings:   append(warnings, ctx.Err().Error()),
			}
		case jobs <- job{index: i, wl: wl}:
		}
	}
	close(jobs)
	wait.Wait()

	out := RightsizingResult{
		Deltas:   make([]RightsizingDelta, 0),
		Scanned:  len(scan),
		Warnings: warnings,
	}
	for _, o := range outcomes {
		out.Deltas = append(out.Deltas, o.deltas...)
		out.Warnings = append(out.Warnings, o.warnings...)
	}
	return out
}

// ApplyRightsizing merges rightsizing findings and suggestions into the report.
func ApplyRightsizing(rep *Report, rs RightsizingResult) {
	if rep == nil {
		return
	}
	if len(rs.Deltas) > 0 {
		rep.Rightsizing = rs.Deltas
	}
	if rs.Skipped {
		msg := rs.SkipReason
		if msg == "" {
			msg = "Rightsizing skipped"
		}
		rep.Sections.Rightsizing = SectionStatus{Status: SectionSkipped, Message: msg}
		rep.Findings = append(rep.Findings, Finding{
			Code:     "optimize.rightsizing.skipped",
			Severity: "medium",
			Title:    "Rightsizing skipped",
			Message:  msg,
		})
		for _, w := range rs.Warnings {
			rep.Findings = append(rep.Findings, Finding{
				Code:     "optimize.rightsizing.warning",
				Severity: "medium",
				Title:    "Prometheus warning",
				Message:  w,
			})
		}
		return
	}

	rep.Sections.Rightsizing = SectionStatus{
		Status: SectionReady,
		Message: fmt.Sprintf(
			"%d rightsizing deltas from %d scanned workloads",
			len(rs.Deltas), rs.Scanned,
		),
	}

	rep.Findings = append(rep.Findings, Finding{
		Code:     "optimize.rightsizing.summary",
		Severity: SeverityInfo,
		Title:    "Rightsizing",
		Message: fmt.Sprintf(
			"%d concrete request/limit deltas over %s (suggestions only — no auto-apply)",
			len(rs.Deltas), rep.Window,
		),
	})

	for _, d := range rs.Deltas {
		rep.Findings = append(rep.Findings, Finding{
			Code:      "optimize.rightsizing.delta",
			Severity:  "low",
			Title:     fmt.Sprintf("%s %s %s", d.Direction, d.Resource, d.Field),
			Message:   d.Message,
			Resource:  fmt.Sprintf("%s/%s", d.Kind, d.Name),
			Namespace: d.Namespace,
		})
		rep.Suggestions = append(rep.Suggestions, Suggestion{
			Code:       "optimize.rightsizing.patch",
			Title:      fmt.Sprintf("%s/%s %s %s", d.Kind, d.Name, d.Resource, d.Field),
			Message:    d.Message,
			ActionHint: "patch-resources",
		})
	}
	for _, w := range rs.Warnings {
		rep.Findings = append(rep.Findings, Finding{
			Code:     "optimize.rightsizing.warning",
			Severity: "medium",
			Title:    "Prometheus warning",
			Message:  w,
		})
	}

	if strings.Contains(rep.Summary, "rightsizing and HPA remain pending") {
		rep.Summary = strings.Replace(
			rep.Summary,
			"rightsizing and HPA remain pending — no mutations.",
			fmt.Sprintf("%d rightsizing suggestions; HPA remains pending — no mutations.", len(rs.Deltas)),
			1,
		)
	} else if strings.Contains(rep.Summary, "Idle, rightsizing, and HPA sections remain pending") {
		rep.Summary = strings.Replace(
			rep.Summary,
			"Idle, rightsizing, and HPA sections remain pending — no mutations.",
			fmt.Sprintf("%d rightsizing suggestions; HPA remains pending — no mutations.", len(rs.Deltas)),
			1,
		)
	} else if len(rs.Deltas) > 0 {
		rep.Summary += fmt.Sprintf(" Proposed %d rightsizing deltas.", len(rs.Deltas))
	}
}

func probeRightsizing(
	ctx context.Context,
	querier toolprometheus.Querier,
	wl Workload,
	window time.Duration,
) ([]RightsizingDelta, []string) {
	ns := strings.TrimSpace(wl.Namespace)
	if ns == "" {
		ns = "default"
	}
	namespace := strconv.Quote(ns)
	podPattern := strconv.Quote("^" + regexp.QuoteMeta(wl.Name) + "-.*")
	podSelector := fmt.Sprintf(`namespace=%s,pod=~%s`, namespace, podPattern)
	win := formatPromWindow(window)
	// Subquery step for quantile_over_time (memory p95).
	step := "1m"
	if window >= 6*time.Hour {
		step = "5m"
	}

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
		{"cpu_limit", fmt.Sprintf(
			`sum(kube_pod_container_resource_limits{%s,resource="cpu",unit="core"})`,
			podSelector,
		)},
		{"memory_p95", fmt.Sprintf(
			`quantile_over_time(0.95, sum(container_memory_working_set_bytes{%s,container!="",container!="POD"})[%s:%s])`,
			podSelector, win, step,
		)},
		{"memory_request", fmt.Sprintf(
			`sum(kube_pod_container_resource_requests{%s,resource="memory",unit="byte"})`,
			podSelector,
		)},
		{"memory_limit", fmt.Sprintf(
			`sum(kube_pod_container_resource_limits{%s,resource="memory",unit="byte"})`,
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

	cpuReq := quantityCores(wl.CPURequest)
	if v, ok := values["cpu_request"]; ok && v > 0 {
		cpuReq = v
	}
	memReq := quantityBytes(wl.MemoryRequest)
	if v, ok := values["memory_request"]; ok && v > 0 {
		memReq = v
	}
	cpuLim := quantityCores(wl.CPULimit)
	if v, ok := values["cpu_limit"]; ok && v > 0 {
		cpuLim = v
	}
	memLim := quantityBytes(wl.MemoryLimit)
	if v, ok := values["memory_limit"]; ok && v > 0 {
		memLim = v
	}

	var deltas []RightsizingDelta
	if cpuUse, ok := values["cpu_usage"]; ok && cpuUse > 0 && cpuReq > 0 {
		suggested := cpuUse / rightsizeCPUTarget
		if deltaSignificant(cpuReq, suggested) {
			cur := formatCPUCores(cpuReq)
			sug := niceCPU(suggested)
			dir := direction(cpuReq, suggested)
			deltas = append(deltas, RightsizingDelta{
				Kind: wl.Kind, Namespace: wl.Namespace, Name: wl.Name,
				Resource: "cpu", Field: "request",
				Current: cur, Suggested: sug, Direction: dir,
				Message: fmt.Sprintf("%s CPU request %s→%s (avg usage targets ~%.0f%% utilization over %s)",
					dir, cur, sug, rightsizeCPUTarget*100, formatWindow(window)),
			})
			// Align limit if present and still far from suggested request.
			if cpuLim > 0 {
				sugLimCores := suggested * rightsizeLimitMult
				if deltaSignificant(cpuLim, sugLimCores) {
					curL := formatCPUCores(cpuLim)
					sugL := niceCPU(sugLimCores)
					dirL := direction(cpuLim, sugLimCores)
					deltas = append(deltas, RightsizingDelta{
						Kind: wl.Kind, Namespace: wl.Namespace, Name: wl.Name,
						Resource: "cpu", Field: "limit",
						Current: curL, Suggested: sugL, Direction: dirL,
						Message: fmt.Sprintf("%s CPU limit %s→%s", dirL, curL, sugL),
					})
				}
			}
		}
	}

	if memP95, ok := values["memory_p95"]; ok && memP95 > 0 && memReq > 0 {
		suggested := memP95 * rightsizeMemHeadroom
		if deltaSignificant(memReq, suggested) {
			cur := formatMemoryBytes(memReq)
			sug := niceMemory(suggested)
			dir := direction(memReq, suggested)
			deltas = append(deltas, RightsizingDelta{
				Kind: wl.Kind, Namespace: wl.Namespace, Name: wl.Name,
				Resource: "memory", Field: "request",
				Current: cur, Suggested: sug, Direction: dir,
				Message: fmt.Sprintf("%s memory request %s→%s (p95 working set + %.0f%% headroom over %s)",
					dir, cur, sug, (rightsizeMemHeadroom-1)*100, formatWindow(window)),
			})
			if memLim > 0 {
				sugLim := suggested * rightsizeLimitMult
				if deltaSignificant(memLim, sugLim) {
					curL := formatMemoryBytes(memLim)
					sugL := niceMemory(sugLim)
					dirL := direction(memLim, sugLim)
					deltas = append(deltas, RightsizingDelta{
						Kind: wl.Kind, Namespace: wl.Namespace, Name: wl.Name,
						Resource: "memory", Field: "limit",
						Current: curL, Suggested: sugL, Direction: dirL,
						Message: fmt.Sprintf("%s memory limit %s→%s", dirL, curL, sugL),
					})
				}
			}
		}
	}

	return deltas, warnings
}

func deltaSignificant(current, suggested float64) bool {
	if current <= 0 || suggested <= 0 {
		return false
	}
	ratio := suggested / current
	if ratio < 1 {
		ratio = current / suggested
	}
	return ratio >= 1+rightsizeMinDelta
}

func direction(current, suggested float64) string {
	if suggested < current {
		return "lower"
	}
	return "raise"
}

func quantityCores(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return float64(q.MilliValue()) / 1000
}

func quantityBytes(s string) float64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	q, err := resource.ParseQuantity(s)
	if err != nil {
		return 0
	}
	return float64(q.Value())
}

func formatCPUCores(cores float64) string {
	return niceCPU(cores)
}

func formatMemoryBytes(bytes float64) string {
	return niceMemory(bytes)
}

func niceCPU(cores float64) string {
	if cores <= 0 || math.IsNaN(cores) || math.IsInf(cores, 0) {
		return "10m"
	}
	mc := math.Ceil(cores * 1000)
	if mc < 10 {
		mc = 10
	}
	mc = math.Ceil(mc/10) * 10
	if mc >= 1000 && int64(mc)%1000 == 0 {
		return strconv.FormatInt(int64(mc)/1000, 10)
	}
	return fmt.Sprintf("%dm", int64(mc))
}

func niceMemory(bytes float64) string {
	if bytes <= 0 || math.IsNaN(bytes) || math.IsInf(bytes, 0) {
		return "16Mi"
	}
	mi := math.Ceil(bytes / (1024 * 1024))
	steps := []float64{16, 32, 64, 128, 256, 512, 768, 1024, 1536, 2048, 3072, 4096, 6144, 8192}
	for _, s := range steps {
		if mi <= s {
			return fmt.Sprintf("%dMi", int64(s))
		}
	}
	rounded := math.Ceil(mi/512) * 512
	return fmt.Sprintf("%dMi", int64(rounded))
}
