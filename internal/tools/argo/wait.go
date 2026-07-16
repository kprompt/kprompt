package argo

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

const DefaultWaitTimeout = 5 * time.Minute

// Wait blocks until the workflow reaches a terminal phase or timeout.
func Wait(ctx context.Context, cfg *rest.Config, namespace, name string, timeout time.Duration, out io.Writer) (WorkflowStatus, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return WorkflowStatus{}, fmt.Errorf("argo dynamic client: %w", err)
	}
	return WaitWithClient(ctx, dc, namespace, name, timeout, out)
}

// WaitWithClient polls workflow status using an injected dynamic client.
func WaitWithClient(ctx context.Context, dc dynamic.Interface, namespace, name string, timeout time.Duration, out io.Writer) (WorkflowStatus, error) {
	if timeout <= 0 {
		timeout = DefaultWaitTimeout
	}
	ns := namespace
	if ns == "" {
		ns = "default"
	}
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if out != nil {
		fmt.Fprintf(out, "Waiting for Workflow/%s -n %s (timeout %s)…\n", name, ns, timeout)
	}

	last, err := GetStatusWithClient(waitCtx, dc, ns, name)
	if err != nil {
		return last, err
	}
	if done, err := finishWait(out, ns, name, last); done {
		return last, err
	}

	watcher, err := dc.Resource(WorkflowGVR).Namespace(ns).Watch(waitCtx, metav1.ListOptions{
		FieldSelector: "metadata.name=" + name,
	})
	if err != nil {
		return pollWorkflowWithClient(waitCtx, dc, ns, name, timeout, out)
	}
	defer watcher.Stop()

	for {
		select {
		case <-waitCtx.Done():
			if errors.Is(waitCtx.Err(), context.DeadlineExceeded) {
				return last, timeoutWorkflowErr(name, timeout, last)
			}
			return last, waitCtx.Err()
		case ev, ok := <-watcher.ResultChan():
			if !ok {
				return pollWorkflowWithClient(waitCtx, dc, ns, name, timeout, out)
			}
			obj, ok := ev.Object.(*unstructured.Unstructured)
			if !ok {
				continue
			}
			last = StatusFromObject(obj)
			if done, err := finishWait(out, ns, name, last); done {
				return last, err
			}
		}
	}
}

func pollWorkflowWithClient(ctx context.Context, dc dynamic.Interface, ns, name string, timeout time.Duration, out io.Writer) (WorkflowStatus, error) {
	ticker := time.NewTicker(750 * time.Millisecond)
	defer ticker.Stop()

	var last WorkflowStatus
	for {
		st, err := GetStatusWithClient(ctx, dc, ns, name)
		if err != nil {
			if ctx.Err() != nil {
				return last, ctx.Err()
			}
			return last, err
		}
		last = st
		if done, err := finishWait(out, ns, name, last); done {
			return last, err
		}
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				return last, timeoutWorkflowErr(name, timeout, last)
			}
			return last, ctx.Err()
		case <-ticker.C:
		}
	}
}

func finishWait(out io.Writer, ns, name string, st WorkflowStatus) (done bool, err error) {
	if !IsTerminalPhase(st.Phase) {
		return false, nil
	}
	if out != nil {
		fmt.Fprintf(out, "✓ %s\n", st.Label())
	}
	if st.Phase == "Failed" || st.Phase == "Error" {
		return true, fmt.Errorf("workflow %s/%s finished with phase %s", ns, name, st.Phase)
	}
	return true, nil
}

func timeoutWorkflowErr(name string, timeout time.Duration, last WorkflowStatus) error {
	return fmt.Errorf("timed out waiting for Workflow/%s after %s (phase=%s)", name, timeout, last.Phase)
}
