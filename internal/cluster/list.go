package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// Query describes a read-only list/get against the cluster.
type Query struct {
	Kind          string // Pod, Deployment, Service, or other Kind
	Namespace     string
	Name          string // optional exact name
	LabelSelector string
	MinMemory     resource.Quantity // optional pod memory filter (requests sum)
	Group         string
	Resource      string // plural API resource name
	Limit         int64
	Continue      string
	Timeout       time.Duration
}

// Row is one printable result line.
type Row struct {
	Namespace string
	Name      string
	Ready     string
	Status    string
	Extra     string // resource-specific trailing columns (tab-separated)
}

// Result is a tabular query outcome.
type Result struct {
	Kind    string
	Headers []string
	Rows    []Row
}

// Reader performs get/list queries.
type Reader struct {
	Client kubernetes.Interface
}

// List runs a Query and returns tabular rows.
func (r *Reader) List(ctx context.Context, q Query) (Result, error) {
	ns := q.Namespace
	if ns == "" {
		ns = "default"
	}
	kind := NormalizeKind(q.Kind)
	switch kind {
	case "Pod":
		return r.listPods(ctx, ns, q)
	case "Deployment":
		return r.listDeployments(ctx, ns, q)
	case "Service":
		return r.listServices(ctx, ns, q)
	case "Node", "ConfigMap", "Secret", "Workflow":
		return Result{}, fmt.Errorf(
			"read of %q is accepted by the generic read contract but not executed yet — waiting on discovery/dynamic client (T-050); currently listed: Pod, Deployment, Service",
			kind,
		)
	default:
		return Result{}, fmt.Errorf(
			"unsupported get kind %q — use a discoverable kind/plural/short name or group-qualified form (e.g. deployments.apps); currently listed: Pod, Deployment, Service (T-050 expands this)",
			q.Kind,
		)
	}
}

// ToReadTable maps a legacy Result into the stable ReadTable contract.
func (res Result) ToReadTable(req ReadRequest) ReadTable {
	table := ReadTable{
		Kind:      firstNonEmpty(res.Kind, req.Resource.Kind, req.Resource.Resource),
		Group:     req.Resource.Group,
		Resource:  req.Resource.Resource,
		Namespace: req.Namespace,
		Scope:     req.Resource.Scope,
		Headers:   append([]string(nil), res.Headers...),
		Continue:  req.Continue,
	}
	if req.Resource.Version != "" {
		if req.Resource.Group == "" {
			table.APIVersion = req.Resource.Version
		} else {
			table.APIVersion = req.Resource.Group + "/" + req.Resource.Version
		}
	}
	for _, row := range res.Rows {
		m := map[string]string{}
		for _, h := range res.Headers {
			switch strings.ToUpper(h) {
			case "NAMESPACE":
				m[h] = row.Namespace
			case "NAME":
				m[h] = row.Name
			case "READY":
				m[h] = row.Ready
			case "STATUS":
				m[h] = row.Status
			default:
				if row.Extra != "" {
					m[h] = row.Extra
				}
			}
		}
		if len(m) == 0 {
			m["NAMESPACE"] = row.Namespace
			m["NAME"] = row.Name
			m["READY"] = row.Ready
			m["STATUS"] = row.Status
		}
		table.Rows = append(table.Rows, m)
	}
	return table
}

// NormalizeKind maps common aliases to canonical kinds.
func NormalizeKind(k string) string {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "", "pod", "pods", "po":
		return "Pod"
	case "deployment", "deployments", "deploy", "deploy.apps", "deployments.apps":
		return "Deployment"
	case "service", "services", "svc":
		return "Service"
	case "workflow", "workflows", "wf":
		return "Workflow"
	case "node", "nodes", "no":
		return "Node"
	case "configmap", "configmaps", "cm":
		return "ConfigMap"
	case "secret", "secrets":
		return "Secret"
	default:
		return strings.TrimSpace(k)
	}
}

func (r *Reader) listPods(ctx context.Context, ns string, q Query) (Result, error) {
	res := Result{Kind: "Pod", Headers: []string{"NAMESPACE", "NAME", "READY", "STATUS", "RESTARTS", "MEMORY_REQ"}}
	opts := metav1.ListOptions{LabelSelector: q.LabelSelector}
	if q.Name != "" {
		pod, err := r.Client.CoreV1().Pods(ns).Get(ctx, q.Name, metav1.GetOptions{})
		if err != nil {
			return res, err
		}
		if row, ok := podRow(*pod, q.MinMemory); ok {
			res.Rows = append(res.Rows, row)
		}
		return res, nil
	}
	list, err := r.Client.CoreV1().Pods(ns).List(ctx, opts)
	if err != nil {
		return res, err
	}
	for _, pod := range list.Items {
		if row, ok := podRow(pod, q.MinMemory); ok {
			res.Rows = append(res.Rows, row)
		}
	}
	return res, nil
}

func podRow(pod corev1.Pod, minMem resource.Quantity) (Row, bool) {
	req := podMemoryRequests(pod)
	if !minMem.IsZero() && req.Cmp(minMem) < 0 {
		return Row{}, false
	}
	ready, total := 0, len(pod.Status.ContainerStatuses)
	restarts := int32(0)
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.Ready {
			ready++
		}
		restarts += cs.RestartCount
	}
	mem := req.String()
	if req.IsZero() {
		mem = "-"
	}
	return Row{
		Namespace: pod.Namespace,
		Name:      pod.Name,
		Ready:     fmt.Sprintf("%d/%d", ready, total),
		Status:    string(pod.Status.Phase),
		Extra:     fmt.Sprintf("%d\t%s", restarts, mem),
	}, true
}

func podMemoryRequests(pod corev1.Pod) resource.Quantity {
	total := resource.NewQuantity(0, resource.BinarySI)
	for _, c := range pod.Spec.Containers {
		if q, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
			total.Add(q)
		}
	}
	return *total
}

func (r *Reader) listDeployments(ctx context.Context, ns string, q Query) (Result, error) {
	res := Result{Kind: "Deployment", Headers: []string{"NAMESPACE", "NAME", "READY", "UP-TO-DATE", "AVAILABLE"}}
	opts := metav1.ListOptions{LabelSelector: q.LabelSelector}
	if q.Name != "" {
		dep, err := r.Client.AppsV1().Deployments(ns).Get(ctx, q.Name, metav1.GetOptions{})
		if err != nil {
			return res, err
		}
		res.Rows = append(res.Rows, deploymentRow(*dep))
		return res, nil
	}
	list, err := r.Client.AppsV1().Deployments(ns).List(ctx, opts)
	if err != nil {
		return res, err
	}
	for _, dep := range list.Items {
		res.Rows = append(res.Rows, deploymentRow(dep))
	}
	return res, nil
}

func deploymentRow(dep appsv1.Deployment) Row {
	desired := int32(0)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	return Row{
		Namespace: dep.Namespace,
		Name:      dep.Name,
		Ready:     fmt.Sprintf("%d/%d", dep.Status.ReadyReplicas, desired),
		Status:    fmt.Sprintf("%d", dep.Status.UpdatedReplicas),
		Extra:     fmt.Sprintf("%d", dep.Status.AvailableReplicas),
	}
}

func (r *Reader) listServices(ctx context.Context, ns string, q Query) (Result, error) {
	res := Result{Kind: "Service", Headers: []string{"NAMESPACE", "NAME", "TYPE", "CLUSTER-IP", "PORT(S)"}}
	opts := metav1.ListOptions{LabelSelector: q.LabelSelector}
	if q.Name != "" {
		svc, err := r.Client.CoreV1().Services(ns).Get(ctx, q.Name, metav1.GetOptions{})
		if err != nil {
			return res, err
		}
		res.Rows = append(res.Rows, serviceRow(*svc))
		return res, nil
	}
	list, err := r.Client.CoreV1().Services(ns).List(ctx, opts)
	if err != nil {
		return res, err
	}
	for _, svc := range list.Items {
		res.Rows = append(res.Rows, serviceRow(svc))
	}
	return res, nil
}

func serviceRow(svc corev1.Service) Row {
	ports := make([]string, 0, len(svc.Spec.Ports))
	for _, p := range svc.Spec.Ports {
		ports = append(ports, fmt.Sprintf("%d/%s", p.Port, p.Protocol))
	}
	return Row{
		Namespace: svc.Namespace,
		Name:      svc.Name,
		Ready:     string(svc.Spec.Type),
		Status:    svc.Spec.ClusterIP,
		Extra:     strings.Join(ports, ","),
	}
}
