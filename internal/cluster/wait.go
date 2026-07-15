package cluster

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

const DefaultWaitTimeout = 5 * time.Minute

// Waiter polls Deployment rollout readiness.
type Waiter struct {
	Client kubernetes.Interface
	Out    io.Writer
}

// WaitDeployment blocks until the Deployment is rolled out or timeout.
func (w *Waiter) WaitDeployment(ctx context.Context, namespace, name string, timeout time.Duration) error {
	if timeout <= 0 {
		timeout = DefaultWaitTimeout
	}
	ns := namespace
	if ns == "" {
		ns = "default"
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if w.Out != nil {
		fmt.Fprintf(w.Out, "Waiting for Deployment/%s -n %s (timeout %s)…\n", name, ns, timeout)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var last *appsv1.Deployment
	for {
		dep, err := w.Client.AppsV1().Deployments(ns).Get(waitCtx, name, metav1.GetOptions{})
		if err != nil {
			if waitCtx.Err() != nil && !errors.Is(err, context.Canceled) {
				return timeoutErr(name, timeout, last)
			}
			return err
		}
		last = dep
		if deploymentComplete(dep) {
			if w.Out != nil {
				fmt.Fprintf(w.Out, "✓ Deployment/%s ready\n", name)
			}
			return nil
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return timeoutErr(name, timeout, last)
			}
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func timeoutErr(name string, timeout time.Duration, dep *appsv1.Deployment) error {
	if dep == nil {
		return fmt.Errorf("timed out waiting for Deployment/%s after %s", name, timeout)
	}
	return fmt.Errorf("timed out waiting for Deployment/%s after %s (updated=%d available=%d desired=%d)",
		name, timeout, dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas, desiredReplicas(dep))
}

func deploymentComplete(dep *appsv1.Deployment) bool {
	desired := desiredReplicas(dep)
	return dep.Status.UpdatedReplicas == desired &&
		dep.Status.Replicas == desired &&
		dep.Status.AvailableReplicas == desired &&
		dep.Status.ObservedGeneration >= dep.Generation
}

func desiredReplicas(dep *appsv1.Deployment) int32 {
	if dep.Spec.Replicas != nil {
		return *dep.Spec.Replicas
	}
	return 1
}
