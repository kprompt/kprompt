package cluster

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// LogsRequest fetches a short log tail for a Pod or Deployment.
type LogsRequest struct {
	Name      string
	Namespace string
	Kind      string // Pod or Deployment
	Tail      int64  // lines (default 100)
	Container string // optional
}

// LogsResult is a printable log tail.
type LogsResult struct {
	Pod       string
	Namespace string
	Container string
	Tail      int64
	Body      string
}

// LogReader fetches pod logs.
type LogReader struct {
	Client kubernetes.Interface
}

// Logs resolves the target Pod and returns the last Tail lines.
func (r *LogReader) Logs(ctx context.Context, req LogsRequest) (LogsResult, error) {
	ns := req.Namespace
	if ns == "" {
		ns = "default"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return LogsResult{}, fmt.Errorf("logs requires a target name")
	}
	tail := req.Tail
	if tail <= 0 {
		tail = 100
	}
	if tail > 5000 {
		tail = 5000
	}

	kind := NormalizeKind(req.Kind)
	if kind != "Pod" && kind != "Deployment" {
		kind = "Deployment"
	}

	podName := name
	container := req.Container
	if kind == "Deployment" {
		pod, err := r.pickDeploymentPod(ctx, ns, name)
		if err != nil {
			if apierrors.IsNotFound(err) {
				// Fall back to treating the name as a Pod.
				podName = name
			} else {
				return LogsResult{}, err
			}
		} else {
			podName = pod.Name
			if container == "" && len(pod.Spec.Containers) == 1 {
				container = pod.Spec.Containers[0].Name
			}
		}
	}

	opts := &corev1.PodLogOptions{TailLines: &tail}
	if container != "" {
		opts.Container = container
	}
	stream, err := r.Client.CoreV1().Pods(ns).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return LogsResult{}, err
	}
	defer stream.Close()
	body, err := io.ReadAll(stream)
	if err != nil {
		return LogsResult{}, err
	}
	return LogsResult{
		Pod:       podName,
		Namespace: ns,
		Container: container,
		Tail:      tail,
		Body:      string(body),
	}, nil
}

func (r *LogReader) pickDeploymentPod(ctx context.Context, ns, name string) (*corev1.Pod, error) {
	dep, err := r.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	list, err := r.Client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(dep.Spec.Selector),
	})
	if err != nil {
		return nil, err
	}
	if len(list.Items) == 0 {
		return nil, fmt.Errorf("no pods found for Deployment/%s", name)
	}
	pods := append([]corev1.Pod(nil), list.Items...)
	sort.SliceStable(pods, func(i, j int) bool {
		return podPreferScore(pods[i]) > podPreferScore(pods[j])
	})
	return &pods[0], nil
}

func podPreferScore(p corev1.Pod) int {
	score := 0
	switch p.Status.Phase {
	case corev1.PodRunning:
		score += 100
	case corev1.PodPending:
		score += 10
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.Ready {
			score += 5
		}
	}
	return score
}
