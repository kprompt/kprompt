package cluster

import (
	"context"
	"fmt"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DescribeRequest identifies a workload for a compact status summary.
type DescribeRequest struct {
	Name      string
	Namespace string
	Kind      string // Pod or Deployment
}

// DescribeReport is a short, human-readable status card (not full kubectl describe).
type DescribeReport struct {
	Kind      string
	Name      string
	Namespace string
	Status    string
	Lines     []string
}

// Describer builds compact describe output.
type Describer struct {
	Client kubernetes.Interface
}

// Describe returns a compact status summary for a Pod or Deployment.
func (d *Describer) Describe(ctx context.Context, req DescribeRequest) (DescribeReport, error) {
	ns := req.Namespace
	if ns == "" {
		ns = "default"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return DescribeReport{}, fmt.Errorf("describe requires a target name")
	}
	kind := NormalizeKind(req.Kind)
	if kind != "Pod" && kind != "Deployment" {
		kind = "Deployment"
	}

	switch kind {
	case "Deployment":
		dep, err := d.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if apierrors.IsNotFound(err) {
			return d.describePod(ctx, ns, name)
		}
		if err != nil {
			return DescribeReport{}, err
		}
		return describeDeployment(dep), nil
	default:
		return d.describePod(ctx, ns, name)
	}
}

func (d *Describer) describePod(ctx context.Context, ns, name string) (DescribeReport, error) {
	pod, err := d.Client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return DescribeReport{}, err
	}
	return describePod(*pod), nil
}

func describeDeployment(dep *appsv1.Deployment) DescribeReport {
	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	rep := DescribeReport{
		Kind:      "Deployment",
		Name:      dep.Name,
		Namespace: dep.Namespace,
		Status:    fmt.Sprintf("ready %d/%d", dep.Status.ReadyReplicas, desired),
		Lines: []string{
			fmt.Sprintf("replicas:  desired=%d ready=%d updated=%d available=%d",
				desired, dep.Status.ReadyReplicas, dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas),
			fmt.Sprintf("strategy:  %s", deploymentStrategy(dep)),
			fmt.Sprintf("selector:  %s", metav1.FormatLabelSelector(dep.Spec.Selector)),
		},
	}
	if len(dep.Spec.Template.Spec.Containers) > 0 {
		c := dep.Spec.Template.Spec.Containers[0]
		rep.Lines = append(rep.Lines, fmt.Sprintf("image:     %s (%s)", c.Image, c.Name))
	}
	age := "unknown"
	if !dep.CreationTimestamp.IsZero() {
		age = formatAge(dep.CreationTimestamp.Time)
	}
	rep.Lines = append(rep.Lines, fmt.Sprintf("age:       %s", age))
	return rep
}

func describePod(pod corev1.Pod) DescribeReport {
	rep := DescribeReport{
		Kind:      "Pod",
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    string(pod.Status.Phase),
		Lines: []string{
			fmt.Sprintf("node:      %s", emptyDash(pod.Spec.NodeName)),
			fmt.Sprintf("podIP:     %s", emptyDash(pod.Status.PodIP)),
		},
	}
	for _, c := range pod.Spec.Containers {
		ready := "false"
		restarts := int32(0)
		for _, st := range pod.Status.ContainerStatuses {
			if st.Name == c.Name {
				if st.Ready {
					ready = "true"
				}
				restarts = st.RestartCount
				break
			}
		}
		rep.Lines = append(rep.Lines, fmt.Sprintf("container: %s image=%s ready=%s restarts=%d", c.Name, c.Image, ready, restarts))
	}
	age := "unknown"
	if !pod.CreationTimestamp.IsZero() {
		age = formatAge(pod.CreationTimestamp.Time)
	}
	rep.Lines = append(rep.Lines, fmt.Sprintf("age:       %s", age))
	return rep
}

func deploymentStrategy(dep *appsv1.Deployment) string {
	if dep.Spec.Strategy.Type == "" {
		return string(appsv1.RollingUpdateDeploymentStrategyType)
	}
	return string(dep.Spec.Strategy.Type)
}

func emptyDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

func formatAge(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
