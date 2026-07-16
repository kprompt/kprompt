package cluster

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// ExplainRequest identifies a workload to diagnose.
type ExplainRequest struct {
	Name      string
	Namespace string
	Kind      string // Pod or Deployment (default Deployment if ambiguous)
}

// Finding is one diagnosed issue.
type Finding struct {
	Severity  string // info, warning, error
	Code      string
	Message   string
	Container string // optional container name
}

// ExplainReport is the outcome of a read-only investigation (facts + heuristics).
type ExplainReport struct {
	Target    string
	Namespace string
	Kind      string
	Status    string
	Findings  []Finding
	Events    []string
	Summary   string
	// Chain is the Deployment → ReplicaSet → Pod → Events → Logs walk (T-024).
	Chain []ChainStep
	// LogTail is a short log excerpt from the most problematic Pod (when applicable).
	LogTail    string
	LogPod     string
	LogContainer string
}

// ChainStep is one line in the investigation chain.
type ChainStep struct {
	Level  string // Deployment, ReplicaSet, Pod, Events, Logs
	Name   string
	Detail string
}

// Explainer gathers status + events and applies lightweight heuristics.
type Explainer struct {
	Client kubernetes.Interface
}

// Explain diagnoses a named Pod or Deployment.
func (e *Explainer) Explain(ctx context.Context, req ExplainRequest) (ExplainReport, error) {
	ns := req.Namespace
	if ns == "" {
		ns = "default"
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return ExplainReport{}, fmt.Errorf("explain requires a target name")
	}
	kind := NormalizeKind(req.Kind)
	if kind != "Pod" && kind != "Deployment" {
		// Default: try Deployment, else Pod.
		kind = "Deployment"
	}

	switch kind {
	case "Deployment":
		return e.explainDeployment(ctx, ns, name)
	case "Pod":
		return e.explainPod(ctx, ns, name)
	default:
		return e.explainDeployment(ctx, ns, name)
	}
}

func (e *Explainer) explainPod(ctx context.Context, ns, name string) (ExplainReport, error) {
	rep := ExplainReport{Target: name, Namespace: ns, Kind: "Pod"}
	pod, err := e.Client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return rep, err
	}
	rep.Status = string(pod.Status.Phase)
	rep.Findings = diagnosePod(*pod)
	rep.Events = e.recentEvents(ctx, ns, "Pod", name)
	rep.Chain = []ChainStep{{
		Level:  "Pod",
		Name:   name,
		Detail: fmt.Sprintf("phase=%s", pod.Status.Phase),
	}}
	if problemScore(*pod) > 0 {
		if tail, container, err := e.tailPodLogs(ctx, ns, pod.Name, "", explainLogTail); err == nil && strings.TrimSpace(tail) != "" {
			rep.LogPod = pod.Name
			rep.LogContainer = container
			rep.LogTail = tail
			rep.Chain = append(rep.Chain, ChainStep{
				Level:  "Logs",
				Name:   pod.Name,
				Detail: fmt.Sprintf("last %d lines (container=%s)", explainLogTail, firstNonEmpty(container, "(default)")),
			})
		}
	}
	rep.Findings = dedupeFindings(rep.Findings)
	rep.Summary = summarize(rep)
	return rep, nil
}

func diagnosePod(pod corev1.Pod) []Finding {
	var out []Finding
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			reason := cs.State.Waiting.Reason
			msg := cs.State.Waiting.Message
			switch reason {
			case "CrashLoopBackOff":
				out = append(out, Finding{Severity: "error", Code: "CrashLoopBackOff", Container: cs.Name, Message: fmt.Sprintf("container %s is crash-looping: %s", cs.Name, msg)})
			case "ImagePullBackOff", "ErrImagePull":
				out = append(out, Finding{Severity: "error", Code: reason, Container: cs.Name, Message: fmt.Sprintf("container %s cannot pull image: %s", cs.Name, msg)})
			case "CreateContainerConfigError":
				out = append(out, Finding{Severity: "error", Code: reason, Container: cs.Name, Message: fmt.Sprintf("container %s config error: %s", cs.Name, msg)})
			default:
				if reason != "" {
					out = append(out, Finding{Severity: "warning", Code: reason, Container: cs.Name, Message: fmt.Sprintf("container %s waiting: %s", cs.Name, firstNonEmpty(msg, reason))})
				}
			}
		}
		if cs.LastTerminationState.Terminated != nil {
			term := cs.LastTerminationState.Terminated
			if term.Reason == "OOMKilled" {
				out = append(out, Finding{
					Severity:  "error",
					Code:      "OOMKilled",
					Container: cs.Name,
					Message:   fmt.Sprintf("container %s was OOMKilled (exit %d); consider raising memory limits", cs.Name, term.ExitCode),
				})
			} else if term.ExitCode != 0 {
				out = append(out, Finding{
					Severity:  "warning",
					Code:      firstNonEmpty(term.Reason, "Error"),
					Container: cs.Name,
					Message:   fmt.Sprintf("container %s last exit code %d (%s)", cs.Name, term.ExitCode, term.Reason),
				})
			}
		}
		if cs.RestartCount > 0 {
			out = append(out, Finding{
				Severity:  "info",
				Code:      "Restarts",
				Container: cs.Name,
				Message:   fmt.Sprintf("container %s restart count=%d", cs.Name, cs.RestartCount),
			})
		}
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			out = append(out, Finding{Severity: "error", Code: "Unschedulable", Message: cond.Message})
		}
	}
	return out
}

func (e *Explainer) recentEvents(ctx context.Context, ns, kind, name string) []string {
	list, err := e.Client.CoreV1().Events(ns).List(ctx, metav1.ListOptions{
		FieldSelector: fmt.Sprintf("involvedObject.kind=%s,involvedObject.name=%s", kind, name),
	})
	if err != nil {
		return nil
	}
	evs := list.Items
	sort.Slice(evs, func(i, j int) bool {
		return eventTime(evs[i]).After(eventTime(evs[j]))
	})
	const max = 8
	if len(evs) > max {
		evs = evs[:max]
	}
	out := make([]string, 0, len(evs))
	for _, ev := range evs {
		out = append(out, fmt.Sprintf("[%s] %s: %s", ev.Type, ev.Reason, strings.TrimSpace(ev.Message)))
	}
	return out
}

func eventTime(ev corev1.Event) time.Time {
	if !ev.LastTimestamp.IsZero() {
		return ev.LastTimestamp.Time
	}
	if !ev.EventTime.IsZero() {
		return ev.EventTime.Time
	}
	return ev.CreationTimestamp.Time
}

func summarize(rep ExplainReport) string {
	if len(rep.Findings) == 0 {
		return fmt.Sprintf("%s/%s looks healthy (no heuristic issues found). Status: %s", rep.Kind, rep.Target, rep.Status)
	}
	var errors, warnings int
	var top string
	for _, f := range rep.Findings {
		switch f.Severity {
		case "error":
			errors++
			if top == "" {
				top = f.Code
			}
		case "warning":
			warnings++
			if top == "" {
				top = f.Code
			}
		}
	}
	if top == "" {
		top = rep.Findings[0].Code
	}
	return fmt.Sprintf("%s/%s: primary signal %s (%d error, %d warning findings). Status: %s",
		rep.Kind, rep.Target, top, errors, warnings, rep.Status)
}

func dedupeFindings(in []Finding) []Finding {
	seen := map[string]bool{}
	var out []Finding
	for _, f := range in {
		key := f.Code + "|" + f.Message
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, f)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
