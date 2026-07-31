package coordinator

import (
	"context"
	"fmt"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/agent/handoff"
	"github.com/kprompt/kprompt/internal/incident"
)

const (
	defaultMaxPods   = 20
	defaultMaxEvents = 30
)

// KubeProbe is a read-only suspect-namespace verifier (AG-050 / ADR-0017).
// It lists Pods + Events in the suspect namespace only — never mutates.
type KubeProbe struct {
	Client    kubernetes.Interface
	MaxPods   int
	MaxEvents int
}

func (p *KubeProbe) Probe(ctx context.Context, suspectNamespace string, _ handoff.Envelope) (*incident.InvestigationReport, error) {
	if p == nil || p.Client == nil {
		return nil, fmt.Errorf("kube probe: client is required")
	}
	ns := strings.TrimSpace(suspectNamespace)
	if ns == "" {
		return nil, fmt.Errorf("kube probe: suspect namespace is required")
	}
	maxPods := p.MaxPods
	if maxPods <= 0 {
		maxPods = defaultMaxPods
	}
	maxEvents := p.MaxEvents
	if maxEvents <= 0 {
		maxEvents = defaultMaxEvents
	}

	at := time.Now().UTC()
	rep := incident.NewInvestigationReport(ns, at)
	rep.Reasoning = "coordinator-kube-probe"

	pods, err := p.Client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{Limit: int64(maxPods)})
	if err != nil {
		rep.Unknowns = append(rep.Unknowns, fmt.Sprintf("probe: list pods in %s: %v", ns, err))
		rep.Degraded = append(rep.Degraded, "pods")
		rep.Summary = fmt.Sprintf("read-only probe of namespace %q partially failed", ns)
		rep.Confidence = 0.2
		return &rep, nil // soft-fail: still return partial report
	}

	var notReady, restarts int
	var facts []string
	for i, pod := range pods.Items {
		if i >= maxPods {
			break
		}
		ready := podReady(pod)
		if !ready {
			notReady++
		}
		rs := containerRestarts(pod)
		restarts += rs
		ref := &incident.ResourceRef{Kind: "Pod", Name: pod.Name, Namespace: ns, APIVersion: "v1"}
		msg := fmt.Sprintf("phase=%s ready=%v restarts=%d", pod.Status.Phase, ready, rs)
		if !ready || rs > 0 || pod.Status.Phase != corev1.PodRunning {
			rep.Evidence = append(rep.Evidence, incident.EvidenceRef{
				Type:      incident.EvidenceObject,
				Resource:  ref,
				Reason:    string(pod.Status.Phase),
				Message:   msg,
				Source:    "coordinator-kube-probe",
				Timestamp: &at,
			})
		}
		if i < 5 {
			facts = append(facts, pod.Name+":"+msg)
		}
	}

	events, err := p.Client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{Limit: int64(maxEvents)})
	if err != nil {
		rep.Unknowns = append(rep.Unknowns, fmt.Sprintf("probe: list events in %s: %v", ns, err))
		rep.Degraded = append(rep.Degraded, "events")
	} else {
		for i, ev := range events.Items {
			if i >= maxEvents {
				break
			}
			if ev.Type == corev1.EventTypeNormal && !interestingEvent(ev.Reason) {
				continue
			}
			ts := ev.LastTimestamp.Time
			if ts.IsZero() {
				ts = ev.EventTime.Time
			}
			if ts.IsZero() {
				ts = at
			}
			tsCopy := ts.UTC()
			ref := &incident.ResourceRef{
				Kind:      ev.InvolvedObject.Kind,
				Name:      ev.InvolvedObject.Name,
				Namespace: ns,
			}
			rep.Evidence = append(rep.Evidence, incident.EvidenceRef{
				Type:      incident.EvidenceEvent,
				Resource:  ref,
				Reason:    ev.Reason,
				Message:   trimMsg(ev.Message, 200),
				Source:    "coordinator-kube-probe",
				Timestamp: &tsCopy,
			})
			rep.Timeline = append(rep.Timeline, incident.EvidenceRef{
				Type:      incident.EvidenceEvent,
				Resource:  ref,
				Reason:    ev.Reason,
				Message:   trimMsg(ev.Message, 120),
				Source:    "coordinator-kube-probe",
				Timestamp: &tsCopy,
			})
		}
	}

	total := len(pods.Items)
	rep.Facts = fmt.Sprintf("pods=%d notReady=%d restarts=%d; %s", total, notReady, restarts, strings.Join(facts, "; "))
	switch {
	case notReady > 0 || restarts > 0:
		rep.Summary = fmt.Sprintf("namespace %q: %d/%d pods not ready, cumulative restarts=%d", ns, notReady, total, restarts)
		rep.Confidence = 0.55
		rep.Severity = incident.SeverityMedium
		rep.Hypotheses = []incident.Hypothesis{{
			Statement:  fmt.Sprintf("workload instability in namespace %q may explain cross-ns symptoms", ns),
			Confidence: 0.55,
			Primary:    true,
		}}
	case total == 0:
		rep.Summary = fmt.Sprintf("namespace %q: no pods listed (empty or RBAC-limited)", ns)
		rep.Confidence = 0.3
		rep.Unknowns = append(rep.Unknowns, "probe saw zero pods — confirm namespace exists and Role allows list")
	default:
		rep.Summary = fmt.Sprintf("namespace %q: %d pods appear ready (no Warning events sampled)", ns, total)
		rep.Confidence = 0.45
		rep.Hypotheses = []incident.Hypothesis{{
			Statement:  fmt.Sprintf("namespace %q looks healthy from a shallow probe — origin may be local or transient", ns),
			Confidence: 0.45,
			Primary:    true,
		}}
	}
	return &rep, nil
}

func podReady(pod corev1.Pod) bool {
	if pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, c := range pod.Status.Conditions {
		if c.Type == corev1.PodReady {
			return c.Status == corev1.ConditionTrue
		}
	}
	return false
}

func containerRestarts(pod corev1.Pod) int {
	n := 0
	for _, cs := range pod.Status.ContainerStatuses {
		n += int(cs.RestartCount)
	}
	return n
}

func interestingEvent(reason string) bool {
	switch strings.ToLower(reason) {
	case "backoff", "unhealthy", "failed", "failedscheduling", "oomkilling", "killing", "evicted", "pulled", "started":
		return true
	default:
		return false
	}
}

func trimMsg(s string, n int) string {
	s = strings.TrimSpace(s)
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
