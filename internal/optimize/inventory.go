package optimize

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
)

const (
	WorkloadDeployment  = "Deployment"
	WorkloadStatefulSet = "StatefulSet"
)

// Workload is one inventoried Deployment or StatefulSet (T-053).
type Workload struct {
	Kind          string `json:"kind"`
	Namespace     string `json:"namespace"`
	Name          string `json:"name"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	CPURequest    string `json:"cpuRequest,omitempty"`
	CPULimit      string `json:"cpuLimit,omitempty"`
	MemoryRequest string `json:"memoryRequest,omitempty"`
	MemoryLimit   string `json:"memoryLimit,omitempty"`
	Containers    int    `json:"containers"`
	MissingReq    bool   `json:"missingRequests"`
	MissingLim    bool   `json:"missingLimits"`
}

// Inventory is the collected cluster/namespace workload signal set.
type Inventory struct {
	Workloads  []Workload
	Warnings   []string
	Truncated  bool
	Namespaces int
}

// CollectInventory lists Deployments and StatefulSets with replica and resource signals.
// Forbidden kinds are skipped with warnings (existing RBAC boundary).
func CollectInventory(ctx context.Context, client kubernetes.Interface, req Request) (Inventory, error) {
	if client == nil {
		return Inventory{}, fmt.Errorf("kubernetes client is required for optimize inventory")
	}
	ns := strings.TrimSpace(req.Namespace)
	limit := int64(cluster.DefaultReadLimit)
	inv := Inventory{Workloads: make([]Workload, 0, 32)}

	deps, depMore, err := listDeployments(ctx, client, ns, limit)
	if err != nil {
		if apierrors.IsForbidden(err) {
			inv.Warnings = append(inv.Warnings, fmt.Sprintf("skipped Deployments: %v", err))
		} else {
			return Inventory{}, fmt.Errorf("list deployments: %w", err)
		}
	} else {
		inv.Workloads = append(inv.Workloads, deps...)
		if depMore {
			inv.Truncated = true
		}
	}

	remain := limit - int64(len(inv.Workloads))
	if remain > 0 {
		sts, stsMore, err := listStatefulSets(ctx, client, ns, remain)
		if err != nil {
			if apierrors.IsForbidden(err) {
				inv.Warnings = append(inv.Warnings, fmt.Sprintf("skipped StatefulSets: %v", err))
			} else {
				return Inventory{}, fmt.Errorf("list statefulsets: %w", err)
			}
		} else {
			inv.Workloads = append(inv.Workloads, sts...)
			if stsMore {
				inv.Truncated = true
			}
		}
	} else if len(inv.Workloads) > 0 {
		inv.Truncated = true
		inv.Workloads = inv.Workloads[:limit]
	}

	sort.Slice(inv.Workloads, func(i, j int) bool {
		if inv.Workloads[i].Namespace != inv.Workloads[j].Namespace {
			return inv.Workloads[i].Namespace < inv.Workloads[j].Namespace
		}
		if inv.Workloads[i].Kind != inv.Workloads[j].Kind {
			return inv.Workloads[i].Kind < inv.Workloads[j].Kind
		}
		return inv.Workloads[i].Name < inv.Workloads[j].Name
	})

	seen := map[string]struct{}{}
	for _, w := range inv.Workloads {
		seen[w.Namespace] = struct{}{}
	}
	inv.Namespaces = len(seen)
	return inv, nil
}

// ApplyInventory fills the report inventory section from collected signals (read-only).
func ApplyInventory(rep *Report, inv Inventory) {
	if rep == nil {
		return
	}
	rep.Workloads = inv.Workloads

	depN, stsN := 0, 0
	missingReq, missingLim := 0, 0
	for _, w := range inv.Workloads {
		switch w.Kind {
		case WorkloadDeployment:
			depN++
		case WorkloadStatefulSet:
			stsN++
		}
		if w.MissingReq {
			missingReq++
		}
		if w.MissingLim {
			missingLim++
		}
	}

	scopeLabel := "cluster"
	if rep.Scope == ScopeNamespace && rep.Namespace != "" {
		scopeLabel = fmt.Sprintf("namespace %q", rep.Namespace)
	}
	rep.Summary = fmt.Sprintf(
		"Optimize inventory for %s: %d workloads (%d Deployments, %d StatefulSets) across %d namespaces (window %s). Idle, rightsizing, and HPA sections remain pending — no mutations.",
		scopeLabel, len(inv.Workloads), depN, stsN, inv.Namespaces, rep.Window,
	)

	findings := []Finding{{
		Code:     "optimize.readonly",
		Severity: SeverityInfo,
		Title:    "Read-only optimize report",
		Message:  "This report never applies changes. Optional fix plans require explicit approval in a later task.",
	}, {
		Code:     "optimize.inventory.summary",
		Severity: SeverityInfo,
		Title:    "Workload inventory",
		Message: fmt.Sprintf(
			"%d workloads inventoried (%d Deployments, %d StatefulSets) in %d namespaces",
			len(inv.Workloads), depN, stsN, inv.Namespaces,
		),
	}}

	if missingReq > 0 {
		findings = append(findings, Finding{
			Code:     "optimize.inventory.missing_requests",
			Severity: "low",
			Title:    "Missing resource requests",
			Message:  fmt.Sprintf("%d workloads have at least one container without CPU or memory requests", missingReq),
		})
	}
	if missingLim > 0 {
		findings = append(findings, Finding{
			Code:     "optimize.inventory.missing_limits",
			Severity: "low",
			Title:    "Missing resource limits",
			Message:  fmt.Sprintf("%d workloads have at least one container without CPU or memory limits", missingLim),
		})
	}
	if inv.Truncated {
		findings = append(findings, Finding{
			Code:     "optimize.inventory.truncated",
			Severity: "medium",
			Title:    "Inventory truncated",
			Message:  fmt.Sprintf("Listed first %d workloads (read limit %d); results may be incomplete", len(inv.Workloads), cluster.DefaultReadLimit),
		})
	}
	for _, w := range inv.Warnings {
		findings = append(findings, Finding{
			Code:     "optimize.inventory.rbac",
			Severity: "medium",
			Title:    "Skipped resource (RBAC)",
			Message:  w,
		})
	}
	rep.Findings = findings

	status := SectionReady
	msg := fmt.Sprintf("%d workloads with replicas and requests/limits", len(inv.Workloads))
	if len(inv.Workloads) == 0 && len(inv.Warnings) > 0 {
		status = SectionSkipped
		msg = "No workloads collected; see RBAC warnings"
	} else if len(inv.Workloads) == 0 {
		msg = "No Deployments or StatefulSets found in scope"
	}
	rep.Sections.Inventory = SectionStatus{Status: status, Message: msg}
}

func listDeployments(ctx context.Context, client kubernetes.Interface, ns string, limit int64) ([]Workload, bool, error) {
	list, err := client.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{Limit: limit})
	if err != nil {
		return nil, false, err
	}
	out := make([]Workload, 0, len(list.Items))
	for _, dep := range list.Items {
		out = append(out, workloadFromDeployment(dep))
		if int64(len(out)) >= limit {
			break
		}
	}
	more := list.Continue != "" || int64(len(list.Items)) > limit
	return out, more, nil
}

func listStatefulSets(ctx context.Context, client kubernetes.Interface, ns string, limit int64) ([]Workload, bool, error) {
	list, err := client.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{Limit: limit})
	if err != nil {
		return nil, false, err
	}
	out := make([]Workload, 0, len(list.Items))
	for _, sts := range list.Items {
		out = append(out, workloadFromStatefulSet(sts))
		if int64(len(out)) >= limit {
			break
		}
	}
	more := list.Continue != "" || int64(len(list.Items)) > limit
	return out, more, nil
}

func workloadFromDeployment(dep appsv1.Deployment) Workload {
	replicas := int32(0)
	if dep.Spec.Replicas != nil {
		replicas = *dep.Spec.Replicas
	}
	res := sumPodTemplateResources(dep.Spec.Template)
	return Workload{
		Kind:          WorkloadDeployment,
		Namespace:     dep.Namespace,
		Name:          dep.Name,
		Replicas:      replicas,
		ReadyReplicas: dep.Status.ReadyReplicas,
		CPURequest:    quantityOrEmpty(res.cpuReq),
		CPULimit:      quantityOrEmpty(res.cpuLim),
		MemoryRequest: quantityOrEmpty(res.memReq),
		MemoryLimit:   quantityOrEmpty(res.memLim),
		Containers:    res.containers,
		MissingReq:    res.missingReq,
		MissingLim:    res.missingLim,
	}
}

func workloadFromStatefulSet(sts appsv1.StatefulSet) Workload {
	replicas := int32(0)
	if sts.Spec.Replicas != nil {
		replicas = *sts.Spec.Replicas
	}
	res := sumPodTemplateResources(sts.Spec.Template)
	return Workload{
		Kind:          WorkloadStatefulSet,
		Namespace:     sts.Namespace,
		Name:          sts.Name,
		Replicas:      replicas,
		ReadyReplicas: sts.Status.ReadyReplicas,
		CPURequest:    quantityOrEmpty(res.cpuReq),
		CPULimit:      quantityOrEmpty(res.cpuLim),
		MemoryRequest: quantityOrEmpty(res.memReq),
		MemoryLimit:   quantityOrEmpty(res.memLim),
		Containers:    res.containers,
		MissingReq:    res.missingReq,
		MissingLim:    res.missingLim,
	}
}

type resourceTotals struct {
	cpuReq, cpuLim, memReq, memLim resource.Quantity
	containers                     int
	missingReq, missingLim         bool
}

func sumPodTemplateResources(tpl corev1.PodTemplateSpec) resourceTotals {
	var out resourceTotals
	containers := append([]corev1.Container{}, tpl.Spec.Containers...)
	containers = append(containers, tpl.Spec.InitContainers...)
	out.containers = len(tpl.Spec.Containers)
	for _, c := range containers {
		cpuReq, hasCPUReq := c.Resources.Requests[corev1.ResourceCPU]
		memReq, hasMemReq := c.Resources.Requests[corev1.ResourceMemory]
		cpuLim, hasCPULim := c.Resources.Limits[corev1.ResourceCPU]
		memLim, hasMemLim := c.Resources.Limits[corev1.ResourceMemory]
		if !hasCPUReq || !hasMemReq {
			out.missingReq = true
		}
		if !hasCPULim || !hasMemLim {
			out.missingLim = true
		}
		if hasCPUReq {
			out.cpuReq.Add(cpuReq)
		}
		if hasMemReq {
			out.memReq.Add(memReq)
		}
		if hasCPULim {
			out.cpuLim.Add(cpuLim)
		}
		if hasMemLim {
			out.memLim.Add(memLim)
		}
	}
	return out
}

func quantityOrEmpty(q resource.Quantity) string {
	if q.IsZero() {
		return ""
	}
	return q.String()
}
