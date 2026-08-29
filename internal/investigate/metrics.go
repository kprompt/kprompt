package investigate

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/incident"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

// MetricsQuerier is the optional Prometheus adapter for investigate (issue #44).
type MetricsQuerier interface {
	Query(ctx context.Context, promQL string, at time.Time) (toolprometheus.Result, error)
}

// enrichMetrics attaches metric EvidenceRefs when a Querier is configured.
// Missing / failed Prom → degraded "prometheus" — never invents values.
func enrichMetrics(ctx context.Context, q MetricsQuerier, ns, workload string) (
	evidence []incident.EvidenceRef,
	ok bool,
) {
	if q == nil {
		return nil, false
	}
	wl := strings.TrimSpace(workload)
	if wl == "" || strings.TrimSpace(ns) == "" {
		return nil, false
	}
	podRE := "^" + regexp.QuoteMeta(wl) + "-.*"

	specs := []struct {
		reason string
		unit   string
		query  string
	}{
		{
			reason: "cpu_usage",
			unit:   "cores",
			query: fmt.Sprintf(
				`sum(rate(container_cpu_usage_seconds_total{namespace=%q,pod=~%q,container!="",container!="POD"}[5m]))`,
				ns, podRE,
			),
		},
		{
			reason: "memory_working_set",
			unit:   "bytes",
			query: fmt.Sprintf(
				`sum(container_memory_working_set_bytes{namespace=%q,pod=~%q,container!="",container!="POD"})`,
				ns, podRE,
			),
		},
		{
			reason: "restart_rate",
			unit:   "restarts/s",
			query: fmt.Sprintf(
				`sum(rate(kube_pod_container_status_restarts_total{namespace=%q,pod=~%q}[15m]))`,
				ns, podRE,
			),
		},
	}

	now := time.Now().UTC()
	for _, spec := range specs {
		res, err := q.Query(ctx, spec.query, time.Time{})
		if err != nil {
			continue
		}
		val, has, err := toolprometheus.FirstValue(res)
		if err != nil || !has {
			continue
		}
		ts := now
		evidence = append(evidence, incident.EvidenceRef{
			Type:      incident.EvidenceMetric,
			Reason:    spec.reason,
			Message:   fmt.Sprintf("%.4g %s", val, spec.unit),
			Timestamp: &ts,
			Source:    "prometheus",
			URI:       spec.query,
			Resource: &incident.ResourceRef{
				Kind:      "Workload",
				Name:      wl,
				Namespace: ns,
			},
		})
	}
	return evidence, len(evidence) > 0
}
