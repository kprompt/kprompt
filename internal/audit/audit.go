// Package audit runs a read-only security / hygiene scan (S-006 · T-084).
//
// The MVP walks Pod templates for Deployments, StatefulSets, and DaemonSets
// and emits ADR-0014 Investigation findings. It never mutates.
package audit

import (
	"context"
	"fmt"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/incident"
)

// Request scopes a hygiene scan to one namespace or the whole cluster.
type Request struct {
	Namespace string // empty = cluster-wide
	Prompt    string
}

// Analyzer lists workloads and emits Investigation findings.
type Analyzer struct {
	Client kubernetes.Interface
}

type workload struct {
	kind string
	name string
	ns   string
	tpl  corev1.PodTemplateSpec
}

// Run returns hygiene findings for workloads in scope.
func (a *Analyzer) Run(ctx context.Context, req Request) (incident.Investigation, error) {
	if a == nil || a.Client == nil {
		return incident.Investigation{}, fmt.Errorf("audit: client required")
	}
	ns := strings.TrimSpace(req.Namespace)
	out := incident.NewInvestigation(req.Prompt, ns)
	if ns == "" {
		out.Namespace = "all"
		out.Target = &incident.ResourceRef{Kind: "Cluster", Name: "cluster"}
	} else {
		out.Target = &incident.ResourceRef{Kind: "Namespace", Name: ns, Namespace: ns}
	}

	workloads, warnings, err := a.listWorkloads(ctx, ns)
	if err != nil {
		return incident.Investigation{}, err
	}
	for _, w := range warnings {
		out.Degraded = appendUnique(out.Degraded, w)
	}

	for i := range workloads {
		scanWorkload(&out, &workloads[i])
	}

	nFindings := len(out.Findings)
	nWorkloads := len(workloads)
	if ns == "" {
		out.Summary = fmt.Sprintf("%d hygiene finding(s) across %d workload(s) (cluster-wide)", nFindings, nWorkloads)
	} else {
		out.Summary = fmt.Sprintf("%d hygiene finding(s) across %d workload(s) in namespace %s", nFindings, nWorkloads, ns)
	}
	if nFindings == 0 {
		out.Summary += "; no issues matched MVP rules"
		out.Confidence = 0.85
	} else {
		out.Confidence = 0.9
	}
	out.SuggestedPlanHint = "Review findings before hardening; suggested patches are not auto-applied in this MVP."

	sortInvestigation(&out)
	if err := incident.ValidateInvestigation(out); err != nil {
		return out, err
	}
	return out, nil
}

func (a *Analyzer) listWorkloads(ctx context.Context, ns string) ([]workload, []string, error) {
	var (
		out      []workload
		warnings []string
		opts     = metav1.ListOptions{Limit: cluster.DefaultReadLimit}
	)

	deps, err := a.Client.AppsV1().Deployments(ns).List(ctx, opts)
	switch {
	case apierrors.IsForbidden(err):
		warnings = append(warnings, "deployments")
	case err != nil:
		return nil, nil, fmt.Errorf("list deployments: %w", err)
	default:
		for i := range deps.Items {
			d := &deps.Items[i]
			out = append(out, workload{kind: "Deployment", name: d.Name, ns: d.Namespace, tpl: d.Spec.Template})
		}
	}

	sts, err := a.Client.AppsV1().StatefulSets(ns).List(ctx, opts)
	switch {
	case apierrors.IsForbidden(err):
		warnings = append(warnings, "statefulsets")
	case err != nil:
		return nil, nil, fmt.Errorf("list statefulsets: %w", err)
	default:
		for i := range sts.Items {
			s := &sts.Items[i]
			out = append(out, workload{kind: "StatefulSet", name: s.Name, ns: s.Namespace, tpl: s.Spec.Template})
		}
	}

	dss, err := a.Client.AppsV1().DaemonSets(ns).List(ctx, opts)
	switch {
	case apierrors.IsForbidden(err):
		warnings = append(warnings, "daemonsets")
	case err != nil:
		return nil, nil, fmt.Errorf("list daemonsets: %w", err)
	default:
		for i := range dss.Items {
			d := &dss.Items[i]
			out = append(out, workload{kind: "DaemonSet", name: d.Name, ns: d.Namespace, tpl: d.Spec.Template})
		}
	}

	return out, warnings, nil
}

func scanWorkload(out *incident.Investigation, w *workload) {
	if out == nil || w == nil {
		return
	}
	spec := w.tpl.Spec
	podSC := spec.SecurityContext

	if spec.HostNetwork || spec.HostPID || spec.HostIPC {
		parts := make([]string, 0, 3)
		if spec.HostNetwork {
			parts = append(parts, "hostNetwork")
		}
		if spec.HostPID {
			parts = append(parts, "hostPID")
		}
		if spec.HostIPC {
			parts = append(parts, "hostIPC")
		}
		addFinding(out, "Audit.HostNamespace", incident.SeverityHigh,
			fmt.Sprintf("%s/%s shares host namespaces", w.kind, w.name),
			"Pod template enables "+strings.Join(parts, ", "),
			w.kind, w.name, w.ns, "HostNamespace")
	}

	containers := append([]corev1.Container{}, spec.InitContainers...)
	containers = append(containers, spec.Containers...)
	for i := range containers {
		scanContainer(out, w, podSC, &containers[i])
	}
}

func scanContainer(out *incident.Investigation, w *workload, podSC *corev1.PodSecurityContext, c *corev1.Container) {
	if c == nil {
		return
	}
	cname := c.Name
	if cname == "" {
		cname = "(unnamed)"
	}
	csc := c.SecurityContext

	if potentiallyRunsAsRoot(podSC, csc) {
		addFinding(out, "Audit.RunAsRoot", incident.SeverityHigh,
			fmt.Sprintf("%s/%s container %s may run as root", w.kind, w.name, cname),
			"runAsNonRoot is not set to true on the container or pod securityContext",
			w.kind, w.name, w.ns, "RunAsRoot")
	}

	if csc != nil && csc.Privileged != nil && *csc.Privileged {
		addFinding(out, "Audit.Privileged", incident.SeverityCritical,
			fmt.Sprintf("%s/%s container %s is privileged", w.kind, w.name, cname),
			"securityContext.privileged=true",
			w.kind, w.name, w.ns, "Privileged")
	}

	if csc != nil && csc.AllowPrivilegeEscalation != nil && *csc.AllowPrivilegeEscalation {
		addFinding(out, "Audit.PrivilegeEscalation", incident.SeverityMedium,
			fmt.Sprintf("%s/%s container %s allows privilege escalation", w.kind, w.name, cname),
			"securityContext.allowPrivilegeEscalation=true",
			w.kind, w.name, w.ns, "PrivilegeEscalation")
	}

	if csc == nil || csc.ReadOnlyRootFilesystem == nil || !*csc.ReadOnlyRootFilesystem {
		addFinding(out, "Audit.WritableRootFS", incident.SeverityMedium,
			fmt.Sprintf("%s/%s container %s has a writable root filesystem", w.kind, w.name, cname),
			"securityContext.readOnlyRootFilesystem is not set to true",
			w.kind, w.name, w.ns, "WritableRootFS")
	}

	if imageIsLatestOrUntagged(c.Image) {
		addFinding(out, "Audit.LatestTag", incident.SeverityMedium,
			fmt.Sprintf("%s/%s container %s uses a mutable image tag", w.kind, w.name, cname),
			fmt.Sprintf("image %q is untagged or tagged latest", c.Image),
			w.kind, w.name, w.ns, "LatestTag")
	}

	if c.ImagePullPolicy == "" && imageIsLatestOrUntagged(c.Image) {
		addFinding(out, "Audit.MissingImagePullPolicy", incident.SeverityLow,
			fmt.Sprintf("%s/%s container %s omits imagePullPolicy with a mutable tag", w.kind, w.name, cname),
			"imagePullPolicy is empty while the image tag is mutable",
			w.kind, w.name, w.ns, "MissingImagePullPolicy")
	}

	_, hasCPUReq := c.Resources.Requests[corev1.ResourceCPU]
	_, hasMemReq := c.Resources.Requests[corev1.ResourceMemory]
	_, hasCPULim := c.Resources.Limits[corev1.ResourceCPU]
	_, hasMemLim := c.Resources.Limits[corev1.ResourceMemory]
	if !hasCPUReq || !hasMemReq {
		addFinding(out, "Audit.MissingRequests", incident.SeverityMedium,
			fmt.Sprintf("%s/%s container %s is missing CPU/memory requests", w.kind, w.name, cname),
			"resources.requests must set both cpu and memory",
			w.kind, w.name, w.ns, "MissingRequests")
	}
	if !hasCPULim || !hasMemLim {
		addFinding(out, "Audit.MissingLimits", incident.SeverityMedium,
			fmt.Sprintf("%s/%s container %s is missing CPU/memory limits", w.kind, w.name, cname),
			"resources.limits must set both cpu and memory",
			w.kind, w.name, w.ns, "MissingLimits")
	}
}

func potentiallyRunsAsRoot(podSC *corev1.PodSecurityContext, csc *corev1.SecurityContext) bool {
	if csc != nil && csc.RunAsNonRoot != nil {
		return !*csc.RunAsNonRoot
	}
	if podSC != nil && podSC.RunAsNonRoot != nil {
		return !*podSC.RunAsNonRoot
	}
	return true
}

func imageIsLatestOrUntagged(image string) bool {
	image = strings.TrimSpace(image)
	if image == "" {
		return false
	}
	if strings.Contains(image, "@") {
		return false // digest-pinned
	}
	ref := image
	if i := strings.LastIndex(ref, "/"); i >= 0 {
		ref = ref[i+1:]
	}
	idx := strings.LastIndex(ref, ":")
	if idx < 0 {
		return true
	}
	return strings.EqualFold(ref[idx+1:], "latest")
}

func addFinding(out *incident.Investigation, code, severity, title, message, kind, name, ns, reason string) {
	if out == nil {
		return
	}
	out.Findings = append(out.Findings, incident.Finding{
		Code:      code,
		Severity:  severity,
		Title:     title,
		Message:   message,
		Namespace: ns,
		Evidence: []incident.EvidenceRef{{
			Type: incident.EvidenceObject,
			Resource: &incident.ResourceRef{
				Kind:      kind,
				Name:      name,
				Namespace: ns,
			},
			Reason:  reason,
			Message: message,
			Source:  "kubernetes",
		}},
	})
}

func sortInvestigation(out *incident.Investigation) {
	if out == nil {
		return
	}
	sort.SliceStable(out.Findings, func(i, j int) bool {
		if out.Findings[i].Severity != out.Findings[j].Severity {
			return severityRank(out.Findings[i].Severity) > severityRank(out.Findings[j].Severity)
		}
		if out.Findings[i].Code != out.Findings[j].Code {
			return out.Findings[i].Code < out.Findings[j].Code
		}
		return out.Findings[i].Title < out.Findings[j].Title
	})
}

func severityRank(s string) int {
	switch s {
	case incident.SeverityCritical:
		return 5
	case incident.SeverityHigh:
		return 4
	case incident.SeverityMedium:
		return 3
	case incident.SeverityLow:
		return 2
	default:
		return 1
	}
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
