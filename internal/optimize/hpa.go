package optimize

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

const (
	maxHPAScan      = 40
	hpaQueryWorkers = 6
	// Static-replica hint when desired replicas are at least this and no HPA targets the workload.
	staticReplicaMin = 2
)

// HPAHint narrates HPA presence, maxed-out, or static-replica state (T-056).
type HPAHint struct {
	Kind           string `json:"kind"`
	Namespace      string `json:"namespace"`
	Name           string `json:"name"`
	HPAName        string `json:"hpaName,omitempty"`
	HasHPA         bool   `json:"hasHPA"`
	Maxed          bool   `json:"maxed,omitempty"`
	StaticReplicas bool   `json:"staticReplicas,omitempty"`
	Current        *int32 `json:"current,omitempty"`
	Desired        *int32 `json:"desired,omitempty"`
	Max            *int32 `json:"max,omitempty"`
	Replicas       int32  `json:"replicas,omitempty"`
	Message        string `json:"message"`
}

// HPAResult is the HPA / replica hint section payload.
type HPAResult struct {
	Hints      []HPAHint
	WithHPA    int
	Maxed      int
	Static     int
	Scanned    int
	Warnings   []string
	Skipped    bool
	SkipReason string
}

type hpaTargetKey struct {
	Namespace string
	Kind      string
	Name      string
}

type hpaRecord struct {
	Name    string
	Current int32
	Desired int32
	Max     int32
}

// CollectHPAHints combines inventory with Kubernetes HPA objects and optional Prometheus signals.
func CollectHPAHints(
	ctx context.Context,
	client kubernetes.Interface,
	querier toolprometheus.Querier,
	workloads []Workload,
	namespace string,
) HPAResult {
	if client == nil {
		return HPAResult{
			Skipped:    true,
			SkipReason: "Kubernetes client is required for HPA hints",
		}
	}
	if len(workloads) == 0 {
		return HPAResult{Hints: []HPAHint{}, Scanned: 0}
	}

	ns := strings.TrimSpace(namespace)
	hpas, warnings, err := listHPAs(ctx, client, ns)
	if err != nil {
		return HPAResult{
			Skipped:    true,
			SkipReason: fmt.Sprintf("could not list HorizontalPodAutoscalers: %v", err),
			Warnings:   warnings,
		}
	}

	scan := workloads
	if len(scan) > maxHPAScan {
		warnings = append(warnings, fmt.Sprintf(
			"HPA hint scan limited to first %d of %d workloads",
			maxHPAScan, len(scan),
		))
		scan = scan[:maxHPAScan]
	}

	byTarget := indexHPAs(hpas)
	out := HPAResult{
		Hints:    make([]HPAHint, 0, len(scan)),
		Scanned:  len(scan),
		Warnings: warnings,
	}

	for _, wl := range scan {
		key := hpaTargetKey{
			Namespace: wl.Namespace,
			Kind:      wl.Kind,
			Name:      wl.Name,
		}
		rec, has := byTarget[key]
		hint := HPAHint{
			Kind:      wl.Kind,
			Namespace: wl.Namespace,
			Name:      wl.Name,
			Replicas:  wl.Replicas,
			HasHPA:    has,
		}
		if has {
			out.WithHPA++
			hint.HPAName = rec.Name
			cur, des, max := rec.Current, rec.Desired, rec.Max
			hint.Current, hint.Desired, hint.Max = &cur, &des, &max
			if max > 0 && (cur >= max || des >= max) {
				hint.Maxed = true
				out.Maxed++
				hint.Message = fmt.Sprintf(
					"%s/%s HPA %q is at max (%d/%d current, desired %d)",
					wl.Kind, wl.Name, rec.Name, cur, max, des,
				)
			} else {
				hint.Message = fmt.Sprintf(
					"%s/%s has HPA %q (current %d, desired %d, max %d)",
					wl.Kind, wl.Name, rec.Name, cur, des, max,
				)
			}
			out.Hints = append(out.Hints, hint)
			continue
		}

		// Static replica hint for Deployments/StatefulSets without HPA.
		if wl.Replicas >= staticReplicaMin {
			hint.StaticReplicas = true
			out.Static++
			hint.Message = fmt.Sprintf(
				"%s/%s runs %d static replicas with no HPA — consider autoscaling if traffic varies",
				wl.Kind, wl.Name, wl.Replicas,
			)
			out.Hints = append(out.Hints, hint)
		}
	}

	if querier != nil {
		enrichHPAFromPrometheus(ctx, querier, &out)
	}
	return out
}

// ApplyHPA merges HPA / replica hints into the optimize report.
func ApplyHPA(rep *Report, hpa HPAResult) {
	if rep == nil {
		return
	}
	if len(hpa.Hints) > 0 {
		rep.HPA = hpa.Hints
	}
	if hpa.Skipped {
		msg := hpa.SkipReason
		if msg == "" {
			msg = "HPA hints skipped"
		}
		rep.Sections.HPA = SectionStatus{Status: SectionSkipped, Message: msg}
		rep.Findings = append(rep.Findings, Finding{
			Code:     "optimize.hpa.skipped",
			Severity: "medium",
			Title:    "HPA hints skipped",
			Message:  msg,
		})
		for _, w := range hpa.Warnings {
			rep.Findings = append(rep.Findings, Finding{
				Code:     "optimize.hpa.warning",
				Severity: "medium",
				Title:    "HPA warning",
				Message:  w,
			})
		}
		return
	}

	rep.Sections.HPA = SectionStatus{
		Status: SectionReady,
		Message: fmt.Sprintf(
			"%d with HPA, %d maxed, %d static-replica hints (%d scanned)",
			hpa.WithHPA, hpa.Maxed, hpa.Static, hpa.Scanned,
		),
	}

	rep.Findings = append(rep.Findings, Finding{
		Code:     "optimize.hpa.summary",
		Severity: SeverityInfo,
		Title:    "HPA / replicas",
		Message: fmt.Sprintf(
			"%d workloads with HPA, %d at max replicas, %d static-replica hints",
			hpa.WithHPA, hpa.Maxed, hpa.Static,
		),
	})

	for _, h := range hpa.Hints {
		severity := SeverityInfo
		code := "optimize.hpa.present"
		title := "HPA present"
		hint := "hpa"
		switch {
		case h.Maxed:
			severity = "medium"
			code = "optimize.hpa.maxed"
			title = "HPA at max replicas"
			hint = "scale"
		case h.StaticReplicas:
			severity = "low"
			code = "optimize.hpa.static"
			title = "Static replicas (no HPA)"
			hint = "hpa"
		}
		rep.Findings = append(rep.Findings, Finding{
			Code:      code,
			Severity:  severity,
			Title:     title,
			Message:   h.Message,
			Resource:  fmt.Sprintf("%s/%s", h.Kind, h.Name),
			Namespace: h.Namespace,
		})
		if h.Maxed || h.StaticReplicas {
			rep.Suggestions = append(rep.Suggestions, Suggestion{
				Code:       code,
				Title:      title,
				Message:    h.Message,
				ActionHint: hint,
			})
		}
	}
	for _, w := range hpa.Warnings {
		rep.Findings = append(rep.Findings, Finding{
			Code:     "optimize.hpa.warning",
			Severity: "medium",
			Title:    "HPA warning",
			Message:  w,
		})
	}

	replace := fmt.Sprintf(
		"%d HPA / replica hints — no mutations.",
		len(hpa.Hints),
	)
	switch {
	case strings.Contains(rep.Summary, "HPA remains pending — no mutations."):
		rep.Summary = strings.Replace(rep.Summary, "HPA remains pending — no mutations.", replace, 1)
	case strings.Contains(rep.Summary, "rightsizing and HPA remain pending — no mutations."):
		rep.Summary = strings.Replace(rep.Summary, "rightsizing and HPA remain pending — no mutations.", replace, 1)
	case strings.Contains(rep.Summary, "Idle, rightsizing, and HPA sections remain pending — no mutations."):
		rep.Summary = strings.Replace(rep.Summary, "Idle, rightsizing, and HPA sections remain pending — no mutations.", replace, 1)
	case len(hpa.Hints) > 0:
		rep.Summary += fmt.Sprintf(" %d HPA / replica hints.", len(hpa.Hints))
	}
}

func listHPAs(ctx context.Context, client kubernetes.Interface, ns string) ([]autoscalingv2.HorizontalPodAutoscaler, []string, error) {
	var warnings []string
	list, err := client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		return list.Items, warnings, nil
	}
	// Fall back to v1 if v2 is unavailable.
	if apierrors.IsNotFound(err) || apierrors.IsForbidden(err) {
		v1list, v1err := client.AutoscalingV1().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
		if v1err != nil {
			if apierrors.IsForbidden(v1err) {
				warnings = append(warnings, fmt.Sprintf("skipped HPAs: %v", v1err))
				return nil, warnings, nil
			}
			return nil, warnings, v1err
		}
		out := make([]autoscalingv2.HorizontalPodAutoscaler, 0, len(v1list.Items))
		for _, h := range v1list.Items {
			out = append(out, autoscalingv2.HorizontalPodAutoscaler{
				ObjectMeta: h.ObjectMeta,
				Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
					ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
						Kind: h.Spec.ScaleTargetRef.Kind,
						Name: h.Spec.ScaleTargetRef.Name,
					},
					MinReplicas: h.Spec.MinReplicas,
					MaxReplicas: h.Spec.MaxReplicas,
				},
				Status: autoscalingv2.HorizontalPodAutoscalerStatus{
					CurrentReplicas: h.Status.CurrentReplicas,
					DesiredReplicas: h.Status.DesiredReplicas,
				},
			})
		}
		if apierrors.IsForbidden(err) {
			warnings = append(warnings, fmt.Sprintf("autoscaling/v2 forbidden, used v1: %v", err))
		}
		return out, warnings, nil
	}
	return nil, warnings, err
}

func indexHPAs(items []autoscalingv2.HorizontalPodAutoscaler) map[hpaTargetKey]hpaRecord {
	out := make(map[hpaTargetKey]hpaRecord, len(items))
	for _, h := range items {
		kind := h.Spec.ScaleTargetRef.Kind
		if kind == "" {
			continue
		}
		// Normalize common scale target kinds.
		switch strings.ToLower(kind) {
		case "deployment":
			kind = WorkloadDeployment
		case "statefulset":
			kind = WorkloadStatefulSet
		}
		key := hpaTargetKey{
			Namespace: h.Namespace,
			Kind:      kind,
			Name:      h.Spec.ScaleTargetRef.Name,
		}
		out[key] = hpaRecord{
			Name:    h.Name,
			Current: h.Status.CurrentReplicas,
			Desired: h.Status.DesiredReplicas,
			Max:     h.Spec.MaxReplicas,
		}
	}
	return out
}

// enrichHPAFromPrometheus overlays T-032-style HPA metrics when kube status is empty/zero.
func enrichHPAFromPrometheus(ctx context.Context, querier toolprometheus.Querier, result *HPAResult) {
	if result == nil || querier == nil {
		return
	}
	type job struct{ index int }
	jobs := make(chan job)
	var wait sync.WaitGroup
	var mu sync.Mutex

	workers := hpaQueryWorkers
	if len(result.Hints) < workers {
		workers = len(result.Hints)
	}
	if workers == 0 {
		return
	}
	for w := 0; w < workers; w++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for j := range jobs {
				h := result.Hints[j.index]
				if !h.HasHPA {
					continue
				}
				cur, des, max, warns := queryHPAMetrics(ctx, querier, h)
				mu.Lock()
				result.Warnings = append(result.Warnings, warns...)
				if cur != nil {
					result.Hints[j.index].Current = cur
				}
				if des != nil {
					result.Hints[j.index].Desired = des
				}
				if max != nil {
					result.Hints[j.index].Max = max
				}
				// Recompute maxed / message if Prom filled gaps.
				hh := &result.Hints[j.index]
				if hh.Max != nil && *hh.Max > 0 {
					c, d := int32(0), int32(0)
					if hh.Current != nil {
						c = *hh.Current
					}
					if hh.Desired != nil {
						d = *hh.Desired
					}
					wasMaxed := hh.Maxed
					hh.Maxed = c >= *hh.Max || d >= *hh.Max
					if hh.Maxed && !wasMaxed {
						result.Maxed++
					}
					hh.Message = fmt.Sprintf(
						"%s/%s HPA %q (current %d, desired %d, max %d)",
						hh.Kind, hh.Name, hh.HPAName, c, d, *hh.Max,
					)
					if hh.Maxed {
						hh.Message = fmt.Sprintf(
							"%s/%s HPA %q is at max (%d/%d current, desired %d)",
							hh.Kind, hh.Name, hh.HPAName, c, *hh.Max, d,
						)
					}
				}
				mu.Unlock()
			}
		}()
	}
	for i := range result.Hints {
		jobs <- job{index: i}
	}
	close(jobs)
	wait.Wait()
}

func queryHPAMetrics(
	ctx context.Context,
	querier toolprometheus.Querier,
	h HPAHint,
) (current, desired, max *int32, warnings []string) {
	ns := strings.TrimSpace(h.Namespace)
	if ns == "" {
		ns = "default"
	}
	namespace := strconv.Quote(ns)
	hpaName := h.HPAName
	if hpaName == "" {
		hpaName = h.Name
	}
	hpaPattern := strconv.Quote("^" + regexp.QuoteMeta(hpaName) + ".*")
	queries := []struct {
		key   string
		query string
	}{
		{"hpa_current", fmt.Sprintf(
			`kube_horizontalpodautoscaler_status_current_replicas{namespace=%s,horizontalpodautoscaler=~%s}`,
			namespace, hpaPattern,
		)},
		{"hpa_desired", fmt.Sprintf(
			`kube_horizontalpodautoscaler_status_desired_replicas{namespace=%s,horizontalpodautoscaler=~%s}`,
			namespace, hpaPattern,
		)},
		{"hpa_max", fmt.Sprintf(
			`kube_horizontalpodautoscaler_spec_max_replicas{namespace=%s,horizontalpodautoscaler=~%s}`,
			namespace, hpaPattern,
		)},
	}
	values := map[string]float64{}
	for _, q := range queries {
		result, err := querier.Query(ctx, q.query, time.Time{})
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("%s/%s %s: %v", h.Kind, h.Name, q.key, err))
			continue
		}
		value, ok, valueErr := toolprometheus.FirstValue(result)
		if valueErr != nil {
			warnings = append(warnings, fmt.Sprintf("%s/%s %s: %v", h.Kind, h.Name, q.key, valueErr))
			continue
		}
		if ok {
			values[q.key] = value
		}
	}
	if v, ok := values["hpa_current"]; ok {
		i := int32(v)
		current = &i
	}
	if v, ok := values["hpa_desired"]; ok {
		i := int32(v)
		desired = &i
	}
	if v, ok := values["hpa_max"]; ok {
		i := int32(v)
		max = &i
	}
	return current, desired, max, warnings
}
