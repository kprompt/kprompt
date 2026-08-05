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
		name, timeout, dep.Status.UpdatedReplicas, dep.Status.AvailableReplicas, DesiredReplicas(dep))
}

// DeploymentRolledOut reports whether a Deployment has finished rolling out.
func DeploymentRolledOut(dep *appsv1.Deployment) bool {
	desired := DesiredReplicas(dep)
	return dep.Status.UpdatedReplicas == desired &&
		dep.Status.Replicas == desired &&
		dep.Status.AvailableReplicas == desired &&
		dep.Status.ObservedGeneration >= dep.Generation
}

// DesiredReplicas returns the Deployment's desired replica count.
func DesiredReplicas(dep *appsv1.Deployment) int32 {
	if dep.Spec.Replicas != nil {
		return *dep.Spec.Replicas
	}
	return 1
}

func deploymentComplete(dep *appsv1.Deployment) bool {
	return DeploymentRolledOut(dep)
}

func desiredReplicas(dep *appsv1.Deployment) int32 {
	return DesiredReplicas(dep)
}

// WaitStatefulSet blocks until the StatefulSet is rolled out or timeout.
func (w *Waiter) WaitStatefulSet(ctx context.Context, namespace, name string, timeout time.Duration) error {
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
		fmt.Fprintf(w.Out, "Waiting for StatefulSet/%s -n %s (timeout %s)…\n", name, ns, timeout)
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	var last *appsv1.StatefulSet
	for {
		sts, err := w.Client.AppsV1().StatefulSets(ns).Get(waitCtx, name, metav1.GetOptions{})
		if err != nil {
			if waitCtx.Err() != nil && !errors.Is(err, context.Canceled) {
				return timeoutSTSErr(name, timeout, last)
			}
			return err
		}
		last = sts
		if statefulSetRolledOut(sts) {
			if w.Out != nil {
				fmt.Fprintf(w.Out, "✓ StatefulSet/%s ready\n", name)
			}
			return nil
		}
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return timeoutSTSErr(name, timeout, last)
			}
			return waitCtx.Err()
		case <-ticker.C:
		}
	}
}

func timeoutSTSErr(name string, timeout time.Duration, sts *appsv1.StatefulSet) error {
	if sts == nil {
		return fmt.Errorf("timed out waiting for StatefulSet/%s after %s", name, timeout)
	}
	return fmt.Errorf("timed out waiting for StatefulSet/%s after %s (updated=%d ready=%d desired=%d)",
		name, timeout, sts.Status.UpdatedReplicas, sts.Status.ReadyReplicas, DesiredSTSReplicas(sts))
}

// StatefulSetRolledOut reports whether a StatefulSet has finished rolling out.
func statefulSetRolledOut(sts *appsv1.StatefulSet) bool {
	desired := DesiredSTSReplicas(sts)
	return sts.Status.ObservedGeneration >= sts.Generation &&
		sts.Status.UpdatedReplicas == desired &&
		sts.Status.ReadyReplicas == desired
}

// DesiredSTSReplicas returns the StatefulSet's desired replica count.
func DesiredSTSReplicas(sts *appsv1.StatefulSet) int32 {
	if sts.Spec.Replicas != nil {
		return *sts.Spec.Replicas
	}
	return 1
}
