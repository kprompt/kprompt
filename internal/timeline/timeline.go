// Package timeline builds ordered incident chronologies for a workload (S-004 · T-082).
//
// MVP sources: Kubernetes Events, ReplicaSet/rollout revisions, HPA status.
// Prometheus / OTel / mesh are listed in Investigation.Degraded until later slices.
package timeline

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/incident"
)

const (
	defaultWindow = time.Hour
	maxEntries    = 80
)

// Request identifies a workload whose chronology to build.
type Request struct {
	Name      string
	Namespace string
	Kind      string // Pod, Deployment, StatefulSet, or DaemonSet
	Prompt    string
	Window    time.Duration // lookback; default 1h
}

// Builder collects Events + RS revisions + HPA into Investigation.Timeline.
type Builder struct {
	Client kubernetes.Interface
}

// Run returns an Investigation whose Timeline is time-ordered EvidenceRefs.
func (b *Builder) Run(ctx context.Context, req Request) (incident.Investigation, error) {
	if b == nil || b.Client == nil {
		return incident.Investigation{}, fmt.Errorf("timeline: client required")
	}
	ns := strings.TrimSpace(req.Namespace)
	if ns == "" {
		ns = "default"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return incident.Investigation{}, fmt.Errorf("timeline: target name required")
	}
	kind := normalizeTimelineKind(req.Kind)
	if kind != "Pod" && kind != "Deployment" && kind != "StatefulSet" && kind != "DaemonSet" {
		kind = "Deployment"
	}
	window := req.Window
	if window <= 0 {
		window = defaultWindow
	}
	cutoff := time.Now().UTC().Add(-window)

	out := incident.NewInvestigation(req.Prompt, ns)
	out.Target = &incident.ResourceRef{Kind: kind, Name: name, Namespace: ns}
	out.Degraded = []string{"prometheus", "otel", "mesh"}

	var entries []incident.EvidenceRef
	var findings []incident.Finding

	switch kind {
	case "Pod":
		pod, err := b.Client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return incident.Investigation{}, err
		}
		entries = append(entries, objectStamp("Pod", pod.Name, ns, pod.CreationTimestamp.Time, "created")...)
		evs, n := b.eventsFor(ctx, ns, "Pod", name, cutoff)
		entries = append(entries, evs...)
		if n > 0 {
			findings = append(findings, finding("Timeline.Events", incident.SeverityInfo,
				"Pod events", fmt.Sprintf("%d Event(s) for Pod/%s in last %s", n, name, window)))
		}
	case "Deployment":
		dep, err := b.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return incident.Investigation{}, err
		}
		entries = append(entries, objectStamp("Deployment", dep.Name, ns, dep.CreationTimestamp.Time, "created")...)
		rsEntries, rsFinding := b.replicaSetEntries(ctx, ns, dep, cutoff)
		entries = append(entries, rsEntries...)
		if rsFinding != nil {
			findings = append(findings, *rsFinding)
		}

		evs, n := b.eventsFor(ctx, ns, "Deployment", name, cutoff)
		entries = append(entries, evs...)
		podEvs, pn := b.podEvents(ctx, ns, dep, cutoff)
		entries = append(entries, podEvs...)
		totalEv := n + pn
		if totalEv > 0 {
			findings = append(findings, finding("Timeline.Events", incident.SeverityInfo,
				"Workload events", fmt.Sprintf("%d Event(s) for Deployment/%s (incl. pods) in last %s", totalEv, name, window)))
		}

		hpaEntries, hpaFinding := b.hpaEntries(ctx, ns, name, cutoff)
		entries = append(entries, hpaEntries...)
		if hpaFinding != nil {
			findings = append(findings, *hpaFinding)
		}
	case "StatefulSet":
		sts, err := b.Client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return incident.Investigation{}, err
		}
		entries = append(entries, objectStamp("StatefulSet", sts.Name, ns, sts.CreationTimestamp.Time, "created")...)

		revEntries, revFinding := b.controllerRevisionEntries(ctx, ns, "StatefulSet", sts.Name, string(sts.UID), cutoff)
		entries = append(entries, revEntries...)
		if revFinding != nil {
			findings = append(findings, *revFinding)
		}

		evs, n := b.eventsFor(ctx, ns, "StatefulSet", name, cutoff)
		entries = append(entries, evs...)
		podEvs, pn := b.podEventsForSelector(ctx, ns, sts.Spec.Selector, cutoff)
		entries = append(entries, podEvs...)
		totalEv := n + pn
		if totalEv > 0 {
			findings = append(findings, finding("Timeline.Events", incident.SeverityInfo,
				"Workload events", fmt.Sprintf("%d Event(s) for StatefulSet/%s (incl. pods) in last %s", totalEv, name, window)))
		}
	case "DaemonSet":
		ds, err := b.Client.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return incident.Investigation{}, err
		}
		entries = append(entries, objectStamp("DaemonSet", ds.Name, ns, ds.CreationTimestamp.Time, "created")...)

		revEntries, revFinding := b.controllerRevisionEntries(ctx, ns, "DaemonSet", ds.Name, string(ds.UID), cutoff)
		entries = append(entries, revEntries...)
		if revFinding != nil {
			findings = append(findings, *revFinding)
		}

		evs, n := b.eventsFor(ctx, ns, "DaemonSet", name, cutoff)
		entries = append(entries, evs...)
		podEvs, pn := b.podEventsForSelector(ctx, ns, ds.Spec.Selector, cutoff)
		entries = append(entries, podEvs...)
		totalEv := n + pn
		if totalEv > 0 {
			findings = append(findings, finding("Timeline.Events", incident.SeverityInfo,
				"Workload events", fmt.Sprintf("%d Event(s) for DaemonSet/%s (incl. pods) in last %s", totalEv, name, window)))
		}
	default:
		return incident.Investigation{}, fmt.Errorf("timeline: unsupported kind %q", req.Kind)
	}

	entries = dedupeSortTruncate(entries, cutoff, maxEntries)
	out.Timeline = entries
	out.Evidence = append([]incident.EvidenceRef(nil), entries...)
	out.Findings = findings
	if len(findings) == 0 {
		out.Findings = []incident.Finding{finding("Timeline.Empty", incident.SeverityInfo,
			"No recent chronology", fmt.Sprintf("No Events/RS/HPA signals for %s/%s in last %s", kind, name, window))}
	}
	out.Summary = summarize(kind, name, window, entries, findings)
	out.RootCause = chronologyHint(entries)
	out.Confidence = confidence(entries)

	if err := incident.ValidateInvestigation(out); err != nil {
		return out, err
	}
	return out, nil
}

func (b *Builder) replicaSetEntries(ctx context.Context, ns string, dep *appsv1.Deployment, cutoff time.Time) ([]incident.EvidenceRef, *incident.Finding) {
	sel := labels.Everything().String()
	if x, ok := selectorString(dep.Spec.Selector); ok {
		sel = x
	}
	list, err := b.Client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{LabelSelector: sel})
	if err != nil {
		f := finding("Timeline.ReplicaSetError", incident.SeverityLow, "ReplicaSet list failed", err.Error())
		return nil, &f
	}
	var out []incident.EvidenceRef
	count := 0
	for i := range list.Items {
		rs := &list.Items[i]
		if !ownedBy(rs, dep) {
			continue
		}
		ts := rs.CreationTimestamp.Time
		if ts.Before(cutoff) && rs.Status.Replicas == 0 {
			continue
		}
		rev := rs.Annotations["deployment.kubernetes.io/revision"]
		msg := fmt.Sprintf("ReplicaSet/%s ready=%d/%d", rs.Name, rs.Status.ReadyReplicas, rs.Status.Replicas)
		if rev != "" {
			msg = fmt.Sprintf("revision %s — %s", rev, msg)
		}
		out = append(out, incident.EvidenceRef{
			Type:      incident.EvidenceObject,
			Resource:  &incident.ResourceRef{Kind: "ReplicaSet", Name: rs.Name, Namespace: ns},
			Reason:    "Rollout",
			Message:   msg,
			Timestamp: timePtr(ts),
			Source:    "kubernetes",
		})
		evs, _ := b.eventsFor(ctx, ns, "ReplicaSet", rs.Name, cutoff)
		out = append(out, evs...)
		count++
	}
	if count == 0 {
		return out, nil
	}
	f := finding("Timeline.Rollouts", incident.SeverityInfo,
		"ReplicaSet revisions", fmt.Sprintf("%d ReplicaSet(s) for Deployment/%s", count, dep.Name))
	return out, &f
}

func (b *Builder) hpaEntries(ctx context.Context, ns, deployName string, cutoff time.Time) ([]incident.EvidenceRef, *incident.Finding) {
	list, err := b.Client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		f := finding("Timeline.HPAError", incident.SeverityLow, "HPA list failed", err.Error())
		return nil, &f
	}
	var out []incident.EvidenceRef
	matched := 0
	for i := range list.Items {
		hpa := &list.Items[i]
		ref := hpa.Spec.ScaleTargetRef
		if !strings.EqualFold(ref.Kind, "Deployment") || ref.Name != deployName {
			continue
		}
		matched++
		ts := hpa.CreationTimestamp.Time
		msg := fmt.Sprintf("HPA/%s current=%d desired=%d min=%d max=%d",
			hpa.Name, hpa.Status.CurrentReplicas, hpa.Status.DesiredReplicas,
			ptrInt32(hpa.Spec.MinReplicas, 1), hpa.Spec.MaxReplicas)
		out = append(out, incident.EvidenceRef{
			Type:      incident.EvidenceObject,
			Resource:  &incident.ResourceRef{Kind: "HorizontalPodAutoscaler", Name: hpa.Name, Namespace: ns},
			Reason:    "HPA",
			Message:   msg,
			Timestamp: timePtr(ts),
			Source:    "kubernetes",
		})
		evs, _ := b.eventsFor(ctx, ns, "HorizontalPodAutoscaler", hpa.Name, cutoff)
		out = append(out, evs...)
		for _, c := range hpa.Status.Conditions {
			ct := c.LastTransitionTime.Time
			if ct.IsZero() || ct.Before(cutoff) {
				continue
			}
			out = append(out, incident.EvidenceRef{
				Type:      incident.EvidenceObject,
				Resource:  &incident.ResourceRef{Kind: "HorizontalPodAutoscaler", Name: hpa.Name, Namespace: ns},
				Reason:    string(c.Type) + "/" + string(c.Status),
				Message:   firstNonEmpty(c.Message, c.Reason),
				Timestamp: timePtr(ct),
				Source:    "kubernetes",
			})
		}
	}
	if matched == 0 {
		return out, nil
	}
	f := finding("Timeline.HPA", incident.SeverityInfo,
		"HPA present", fmt.Sprintf("%d HPA target(s) Deployment/%s", matched, deployName))
	return out, &f
}

func (b *Builder) podEvents(ctx context.Context, ns string, dep *appsv1.Deployment, cutoff time.Time) ([]incident.EvidenceRef, int) {
	return b.podEventsForSelector(ctx, ns, dep.Spec.Selector, cutoff)
}

func (b *Builder) podEventsForSelector(ctx context.Context, ns string, sel *metav1.LabelSelector, cutoff time.Time) ([]incident.EvidenceRef, int) {
	labelSelector, ok := selectorString(sel)
	if !ok {
		return nil, 0
	}
	pods, err := b.Client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: labelSelector})
	if err != nil {
		return nil, 0
	}
	var out []incident.EvidenceRef
	total := 0
	limit := len(pods.Items)
	if limit > 10 {
		limit = 10
	}
	for i := 0; i < limit; i++ {
		pod := &pods.Items[i]
		evs, n := b.eventsFor(ctx, ns, "Pod", pod.Name, cutoff)
		out = append(out, evs...)
		total += n
	}
	return out, total
}

func (b *Builder) controllerRevisionEntries(ctx context.Context, ns, ownerKind, ownerName, ownerUID string, cutoff time.Time) ([]incident.EvidenceRef, *incident.Finding) {
	list, err := b.Client.AppsV1().ControllerRevisions(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		f := finding("Timeline.ControllerRevisionError", incident.SeverityLow, "ControllerRevision list failed", err.Error())
		return nil, &f
	}
	var out []incident.EvidenceRef
	count := 0
	for i := range list.Items {
		rev := &list.Items[i]
		if !ownedByControllerRevision(rev, ownerKind, ownerUID) {
			continue
		}
		ts := rev.CreationTimestamp.Time
		if ts.Before(cutoff) {
			continue
		}
		if rev.Revision == 0 {
			continue
		}
		msg := fmt.Sprintf("ControllerRevision/%s revision=%d for %s/%s", rev.Name, rev.Revision, ownerKind, ownerName)
		out = append(out, incident.EvidenceRef{
			Type:      incident.EvidenceObject,
			Resource:  &incident.ResourceRef{Kind: "ControllerRevision", Name: rev.Name, Namespace: ns},
			Reason:    "Rollout",
			Message:   msg,
			Timestamp: timePtr(ts),
			Source:    "kubernetes",
		})
		evs, _ := b.eventsFor(ctx, ns, "ControllerRevision", rev.Name, cutoff)
		out = append(out, evs...)
		count++
	}
	if count == 0 {
		return out, nil
	}
	f := finding("Timeline.ControllerRevisions", incident.SeverityInfo,
		"Controller revisions", fmt.Sprintf("%d ControllerRevision(s) for %s/%s", count, ownerKind, ownerName))
	return out, &f
}
func (b *Builder) eventsFor(ctx context.Context, ns, kind, name string, cutoff time.Time) ([]incident.EvidenceRef, int) {
	list, err := b.Client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s", kind, name),
	})
	if err != nil {
		return nil, 0
	}
	var out []incident.EvidenceRef
	for i := range list.Items {
		ev := &list.Items[i]
		ts := eventTime(ev)
		if !ts.IsZero() && ts.Before(cutoff) {
			continue
		}
		out = append(out, incident.EvidenceRef{
			Type: incident.EvidenceEvent,
			Resource: &incident.ResourceRef{
				Kind: kind, Name: name, Namespace: ns,
			},
			Reason:    ev.Reason,
			Message:   strings.TrimSpace(ev.Message),
			Timestamp: timePtr(ts),
			Source:    "kubernetes",
		})
	}
	return out, len(out)
}

func objectStamp(kind, name, ns string, created time.Time, reason string) []incident.EvidenceRef {
	if created.IsZero() {
		return nil
	}
	return []incident.EvidenceRef{{
		Type:      incident.EvidenceObject,
		Resource:  &incident.ResourceRef{Kind: kind, Name: name, Namespace: ns},
		Reason:    reason,
		Message:   fmt.Sprintf("%s/%s %s", kind, name, reason),
		Timestamp: timePtr(created.UTC()),
		Source:    "kubernetes",
	}}
}

func normalizeTimelineKind(k string) string {
	switch strings.ToLower(strings.TrimSpace(cluster.NormalizeKind(k))) {
	case "", "deployment":
		return "Deployment"
	case "pod":
		return "Pod"
	case "statefulset", "statefulsets", "sts":
		return "StatefulSet"
	case "daemonset", "daemonsets", "ds":
		return "DaemonSet"
	default:
		return strings.TrimSpace(cluster.NormalizeKind(k))
	}
}
func ownedBy(rs *appsv1.ReplicaSet, dep *appsv1.Deployment) bool {
	for _, o := range rs.OwnerReferences {
		if o.UID == dep.UID && o.Kind == "Deployment" {
			return true
		}
	}
	return false
}

func ownedByControllerRevision(rev *appsv1.ControllerRevision, ownerKind, ownerUID string) bool {
	for _, o := range rev.OwnerReferences {
		if o.Kind == ownerKind && string(o.UID) == ownerUID {
			return true
		}
	}
	return false
}

func selectorString(sel *metav1.LabelSelector) (string, bool) {
	if sel == nil {
		return "", false
	}
	x, err := metav1.LabelSelectorAsSelector(sel)
	if err != nil || x.Empty() {
		return "", false
	}
	return x.String(), true
}

func eventTime(ev *corev1.Event) time.Time {
	if ev.LastTimestamp.Time.After(ev.FirstTimestamp.Time) {
		return ev.LastTimestamp.Time.UTC()
	}
	if !ev.FirstTimestamp.IsZero() {
		return ev.FirstTimestamp.Time.UTC()
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time.UTC()
	}
	return ev.CreationTimestamp.Time.UTC()
}

func dedupeSortTruncate(in []incident.EvidenceRef, cutoff time.Time, max int) []incident.EvidenceRef {
	seen := map[string]struct{}{}
	var out []incident.EvidenceRef
	for _, e := range in {
		if e.Timestamp != nil && e.Timestamp.Before(cutoff) && e.Type == incident.EvidenceEvent {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%v|%s", e.Type, e.Reason, e.Message, e.Timestamp, refKey(e.Resource))
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, e)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ti, tj := time.Time{}, time.Time{}
		if out[i].Timestamp != nil {
			ti = *out[i].Timestamp
		}
		if out[j].Timestamp != nil {
			tj = *out[j].Timestamp
		}
		if ti.Equal(tj) {
			return out[i].Reason < out[j].Reason
		}
		return ti.Before(tj)
	})
	if len(out) > max {
		out = out[len(out)-max:]
	}
	return out
}

func refKey(r *incident.ResourceRef) string {
	if r == nil {
		return ""
	}
	return r.Kind + "/" + r.Namespace + "/" + r.Name
}

func summarize(kind, name string, window time.Duration, entries []incident.EvidenceRef, findings []incident.Finding) string {
	ev, obj := 0, 0
	for _, e := range entries {
		switch e.Type {
		case incident.EvidenceEvent:
			ev++
		default:
			obj++
		}
	}
	parts := []string{fmt.Sprintf("Timeline for %s/%s (last %s): %d event(s), %d object stamp(s)", kind, name, window, ev, obj)}
	for _, f := range findings {
		if strings.HasPrefix(f.Code, "Timeline.") && f.Code != "Timeline.Empty" {
			parts = append(parts, f.Message)
		}
	}
	return strings.Join(parts, "; ")
}

func chronologyHint(entries []incident.EvidenceRef) string {
	if len(entries) == 0 {
		return "No chronology signals in window"
	}
	last := entries[len(entries)-1]
	msg := strings.TrimSpace(last.Message)
	if msg == "" {
		msg = last.Reason
	}
	return fmt.Sprintf("Latest: %s", msg)
}

func confidence(entries []incident.EvidenceRef) float64 {
	if len(entries) == 0 {
		return 0.4
	}
	if len(entries) >= 5 {
		return 0.85
	}
	return 0.7
}

func finding(code, sev, title, msg string) incident.Finding {
	return incident.Finding{Code: code, Severity: sev, Title: title, Message: msg}
}

func timePtr(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	u := t.UTC()
	return &u
}

func ptrInt32(p *int32, def int32) int32 {
	if p == nil {
		return def
	}
	return *p
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
