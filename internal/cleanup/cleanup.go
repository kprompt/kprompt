// Package cleanup detects unused / stale Kubernetes resources (S-007 · T-085).
//
// The MVP reports orphaned ConfigMaps/Secrets, completed Jobs, and superseded
// ReplicaSets as ADR-0014 Investigation findings. Approve-gated delete plans may
// remove stale Jobs and ReplicaSets; ConfigMap/Secret orphans stay guidance-only
// unless the prompt confirms orphans (stricter gate via suggest).
package cleanup

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/incident"
)

// DefaultCompletedJobAge is the minimum age before a finished Job is stale.
const DefaultCompletedJobAge = 24 * time.Hour

// Request scopes a cleanup scan to one namespace or the whole cluster.
type Request struct {
	Namespace       string // empty = cluster-wide
	Prompt          string
	CompletedJobAge time.Duration
	Now             time.Time // injectable clock for tests
}

// Analyzer lists resources and reports unused / stale candidates.
type Analyzer struct {
	Client kubernetes.Interface
}

// Run returns cleanup candidates for resources in scope.
func (a *Analyzer) Run(ctx context.Context, req Request) (incident.Investigation, error) {
	if a == nil || a.Client == nil {
		return incident.Investigation{}, fmt.Errorf("cleanup: client required")
	}
	ns := strings.TrimSpace(req.Namespace)
	now := req.Now
	if now.IsZero() {
		now = time.Now()
	}
	jobAge := req.CompletedJobAge
	if jobAge <= 0 {
		jobAge = DefaultCompletedJobAge
	}

	out := incident.NewInvestigation(req.Prompt, ns)
	if ns == "" {
		out.Namespace = "all"
		out.Target = &incident.ResourceRef{Kind: "Cluster", Name: "cluster"}
	} else {
		out.Target = &incident.ResourceRef{Kind: "Namespace", Name: ns, Namespace: ns}
	}

	refs, err := a.collectReferences(ctx, ns, &out)
	if err != nil {
		return incident.Investigation{}, err
	}

	orphans := a.scanConfigMaps(ctx, ns, refs, &out)
	orphans += a.scanSecrets(ctx, ns, refs, &out)
	jobs := a.scanJobs(ctx, ns, now, jobAge, &out)
	rss := a.scanReplicaSets(ctx, ns, &out)
	pvcs := a.scanPVCs(ctx, ns, &out)
	services := a.scanServices(ctx, ns, &out)

	total := orphans + jobs + rss + pvcs + services
	scope := "cluster-wide"
	if ns != "" {
		scope = "namespace " + ns
	}
	out.Summary = fmt.Sprintf(
		"%d cleanup candidate(s) in %s: %d unused ConfigMap/Secret(s), %d completed Job(s), %d superseded ReplicaSet(s), %d unused PVC(s), %d empty Service(s)",
		total, scope, orphans, jobs, rss, pvcs, services,
	)
	if total == 0 {
		out.Summary += "; nothing to clean up"
		out.Confidence = 0.85
	} else {
		out.Confidence = 0.8
	}
	out.SuggestedPlanHint = "Approve-gated delete covers stale Jobs / ReplicaSets; ConfigMap/Secret orphans need a confirm-orphans phrase in the prompt (then DELETE-ORPHANS / --approve)."

	sortInvestigation(&out)
	if err := incident.ValidateInvestigation(out); err != nil {
		return out, err
	}
	return out, nil
}

// references tracks which ConfigMaps/Secrets are consumed by live objects.
type references struct {
	configMaps map[string]struct{}
	secrets    map[string]struct{}
}

func newReferences() references {
	return references{
		configMaps: map[string]struct{}{},
		secrets:    map[string]struct{}{},
	}
}

func (r references) addConfigMap(name string) {
	if name != "" {
		r.configMaps[name] = struct{}{}
	}
}

func (r references) addSecret(name string) {
	if name != "" {
		r.secrets[name] = struct{}{}
	}
}

func (a *Analyzer) collectReferences(ctx context.Context, ns string, out *incident.Investigation) (references, error) {
	refs := newReferences()
	opts := metav1.ListOptions{Limit: cluster.DefaultReadLimit}

	pods, err := a.Client.CoreV1().Pods(ns).List(ctx, opts)
	switch {
	case apierrors.IsForbidden(err):
		out.Degraded = appendUnique(out.Degraded, "pods")
	case err != nil:
		return refs, fmt.Errorf("list pods: %w", err)
	default:
		for i := range pods.Items {
			collectPodSpecRefs(&pods.Items[i].Spec, refs)
		}
	}

	deps, err := a.Client.AppsV1().Deployments(ns).List(ctx, opts)
	if err == nil {
		for i := range deps.Items {
			collectPodSpecRefs(&deps.Items[i].Spec.Template.Spec, refs)
		}
	}
	sts, err := a.Client.AppsV1().StatefulSets(ns).List(ctx, opts)
	if err == nil {
		for i := range sts.Items {
			collectPodSpecRefs(&sts.Items[i].Spec.Template.Spec, refs)
		}
	}
	dss, err := a.Client.AppsV1().DaemonSets(ns).List(ctx, opts)
	if err == nil {
		for i := range dss.Items {
			collectPodSpecRefs(&dss.Items[i].Spec.Template.Spec, refs)
		}
	}

	sas, err := a.Client.CoreV1().ServiceAccounts(ns).List(ctx, opts)
	if err == nil {
		for i := range sas.Items {
			sa := &sas.Items[i]
			for _, s := range sa.Secrets {
				refs.addSecret(s.Name)
			}
			for _, s := range sa.ImagePullSecrets {
				refs.addSecret(s.Name)
			}
		}
	}

	return refs, nil
}

func collectPodSpecRefs(spec *corev1.PodSpec, refs references) {
	if spec == nil {
		return
	}
	for _, s := range spec.ImagePullSecrets {
		refs.addSecret(s.Name)
	}
	containers := append([]corev1.Container{}, spec.InitContainers...)
	containers = append(containers, spec.Containers...)
	for i := range containers {
		c := &containers[i]
		for _, env := range c.Env {
			if env.ValueFrom == nil {
				continue
			}
			if env.ValueFrom.ConfigMapKeyRef != nil {
				refs.addConfigMap(env.ValueFrom.ConfigMapKeyRef.Name)
			}
			if env.ValueFrom.SecretKeyRef != nil {
				refs.addSecret(env.ValueFrom.SecretKeyRef.Name)
			}
		}
		for _, ef := range c.EnvFrom {
			if ef.ConfigMapRef != nil {
				refs.addConfigMap(ef.ConfigMapRef.Name)
			}
			if ef.SecretRef != nil {
				refs.addSecret(ef.SecretRef.Name)
			}
		}
	}
	for _, v := range spec.Volumes {
		if v.ConfigMap != nil {
			refs.addConfigMap(v.ConfigMap.Name)
		}
		if v.Secret != nil {
			refs.addSecret(v.Secret.SecretName)
		}
		if v.Projected != nil {
			for _, src := range v.Projected.Sources {
				if src.ConfigMap != nil {
					refs.addConfigMap(src.ConfigMap.Name)
				}
				if src.Secret != nil {
					refs.addSecret(src.Secret.Name)
				}
			}
		}
	}
}

func (a *Analyzer) scanConfigMaps(ctx context.Context, ns string, refs references, out *incident.Investigation) int {
	list, err := a.Client.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{Limit: cluster.DefaultReadLimit})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "configmaps")
		return 0
	}
	count := 0
	for i := range list.Items {
		cm := &list.Items[i]
		if isSystemConfigMap(cm.Name) {
			continue
		}
		if _, used := refs.configMaps[cm.Name]; used {
			continue
		}
		count++
		addFinding(out, "Cleanup.UnusedConfigMap", incident.SeverityLow,
			fmt.Sprintf("ConfigMap/%s appears unused", cm.Name),
			"No Pod, workload template, or ServiceAccount references this ConfigMap",
			"ConfigMap", cm.Name, cm.Namespace, "Unused")
	}
	return count
}

func (a *Analyzer) scanSecrets(ctx context.Context, ns string, refs references, out *incident.Investigation) int {
	list, err := a.Client.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{Limit: cluster.DefaultReadLimit})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "secrets")
		return 0
	}
	count := 0
	for i := range list.Items {
		sec := &list.Items[i]
		if isSystemSecret(sec) {
			continue
		}
		if _, used := refs.secrets[sec.Name]; used {
			continue
		}
		count++
		addFinding(out, "Cleanup.UnusedSecret", incident.SeverityLow,
			fmt.Sprintf("Secret/%s appears unused", sec.Name),
			"No Pod, workload template, or ServiceAccount references this Secret",
			"Secret", sec.Name, sec.Namespace, "Unused")
	}
	return count
}

func (a *Analyzer) scanJobs(ctx context.Context, ns string, now time.Time, minAge time.Duration, out *incident.Investigation) int {
	list, err := a.Client.BatchV1().Jobs(ns).List(ctx, metav1.ListOptions{Limit: cluster.DefaultReadLimit})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "jobs")
		return 0
	}
	count := 0
	for i := range list.Items {
		job := &list.Items[i]
		finished, when := jobFinished(job)
		if !finished {
			continue
		}
		age := now.Sub(when)
		if when.IsZero() || age < minAge {
			continue
		}
		count++
		addFinding(out, "Cleanup.CompletedJob", incident.SeverityInfo,
			fmt.Sprintf("Job/%s finished %s ago", job.Name, roundDuration(age)),
			"Completed Jobs are not garbage-collected unless ttlSecondsAfterFinished is set",
			"Job", job.Name, job.Namespace, "Completed")
	}
	return count
}

func (a *Analyzer) scanReplicaSets(ctx context.Context, ns string, out *incident.Investigation) int {
	list, err := a.Client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{Limit: cluster.DefaultReadLimit})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "replicasets")
		return 0
	}
	count := 0
	for i := range list.Items {
		rs := &list.Items[i]
		desired := int32(0)
		if rs.Spec.Replicas != nil {
			desired = *rs.Spec.Replicas
		}
		if desired != 0 || rs.Status.Replicas != 0 {
			continue
		}
		owner := ownerName(rs.OwnerReferences, "Deployment")
		if owner == "" {
			continue // bare, scaled-to-zero RS may be intentional
		}
		count++
		addFinding(out, "Cleanup.OldReplicaSet", incident.SeverityInfo,
			fmt.Sprintf("ReplicaSet/%s is scaled to zero", rs.Name),
			fmt.Sprintf("Superseded revision of Deployment/%s; safe to prune beyond revisionHistoryLimit", owner),
			"ReplicaSet", rs.Name, rs.Namespace, "Superseded")
	}
	return count
}

func jobFinished(job *batchv1.Job) (bool, time.Time) {
	if job == nil {
		return false, time.Time{}
	}
	for _, cond := range job.Status.Conditions {
		if (cond.Type == batchv1.JobComplete || cond.Type == batchv1.JobFailed) &&
			cond.Status == corev1.ConditionTrue {
			when := cond.LastTransitionTime.Time
			if job.Status.CompletionTime != nil {
				when = job.Status.CompletionTime.Time
			}
			return true, when
		}
	}
	if job.Status.CompletionTime != nil {
		return true, job.Status.CompletionTime.Time
	}
	return false, time.Time{}
}

func ownerName(owners []metav1.OwnerReference, kind string) string {
	for _, o := range owners {
		if strings.EqualFold(o.Kind, kind) {
			return o.Name
		}
	}
	return ""
}

func isSystemConfigMap(name string) bool {
	return name == "kube-root-ca.crt"
}

func isSystemSecret(sec *corev1.Secret) bool {
	if sec == nil {
		return true
	}
	switch sec.Type {
	case corev1.SecretTypeServiceAccountToken,
		corev1.SecretTypeDockercfg,
		corev1.SecretTypeDockerConfigJson:
		return true
	}
	if _, ok := sec.Annotations["kubernetes.io/service-account.name"]; ok {
		return true
	}
	return false
}

func (a *Analyzer) scanPVCs(ctx context.Context, ns string, out *incident.Investigation) int {
	list, err := a.Client.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{Limit: cluster.DefaultReadLimit})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "persistentvolumeclaims")
		return 0
	}
	count := 0
	for i := range list.Items {
		pvc := &list.Items[i]
		if pvc.Status.Phase == corev1.ClaimBound {
			continue
		}
		count++
		addFinding(out, "Cleanup.UnusedPVC", incident.SeverityLow,
			fmt.Sprintf("PersistentVolumeClaim/%s appears unused", pvc.Name),
			fmt.Sprintf("PersistentVolumeClaim status phase is %q (not Bound)", pvc.Status.Phase),
			"PersistentVolumeClaim", pvc.Name, pvc.Namespace, "Unused")
	}
	return count
}

func (a *Analyzer) scanServices(ctx context.Context, ns string, out *incident.Investigation) int {
	list, err := a.Client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{Limit: cluster.DefaultReadLimit})
	if err != nil {
		out.Degraded = appendUnique(out.Degraded, "services")
		return 0
	}
	count := 0
	for i := range list.Items {
		svc := &list.Items[i]
		if svc.Spec.ClusterIP == corev1.ClusterIPNone {
			continue
		}
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		eps, err := a.Client.CoreV1().Endpoints(svc.Namespace).Get(ctx, svc.Name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				// No endpoints object
			} else {
				continue // Skip if we get transient / forbidden errors
			}
		} else {
			hasAddresses := false
			for _, subset := range eps.Subsets {
				if len(subset.Addresses) > 0 {
					hasAddresses = true
					break
				}
			}
			if hasAddresses {
				continue
			}
		}
		count++
		addFinding(out, "Cleanup.EmptyService", incident.SeverityLow,
			fmt.Sprintf("Service/%s has no active endpoints", svc.Name),
			fmt.Sprintf("Service has a selector but matches zero active pods in namespace %s", svc.Namespace),
			"Service", svc.Name, svc.Namespace, "EmptyService")
	}
	return count
}

func roundDuration(d time.Duration) time.Duration {
	if d >= time.Hour {
		return d.Round(time.Hour)
	}
	return d.Round(time.Minute)
}

func addFinding(out *incident.Investigation, code, severity, title, message, kind, name, ns, reason string) {
	if out == nil {
		return
	}
	ref := incident.ResourceRef{Kind: kind, Name: name, Namespace: ns}
	ev := incident.EvidenceRef{
		Type:     incident.EvidenceObject,
		Resource: &ref,
		Reason:   reason,
		Message:  message,
		Source:   "kubernetes",
	}
	out.Findings = append(out.Findings, incident.Finding{
		Code:      code,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Namespace: ns,
		Evidence:  []incident.EvidenceRef{ev},
	})
}

func sortInvestigation(out *incident.Investigation) {
	if out == nil {
		return
	}
	sort.SliceStable(out.Findings, func(i, j int) bool {
		if out.Findings[i].Code != out.Findings[j].Code {
			return out.Findings[i].Code < out.Findings[j].Code
		}
		return out.Findings[i].Title < out.Findings[j].Title
	})
}

func appendUnique(list []string, item string) []string {
	item = strings.TrimSpace(item)
	if item == "" {
		return list
	}
	for _, existing := range list {
		if existing == item {
			return list
		}
	}
	return append(list, item)
}
