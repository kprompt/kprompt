package cluster

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

const explainLogTail int64 = 40

func (e *Explainer) explainDeployment(ctx context.Context, ns, name string) (ExplainReport, error) {
	rep := ExplainReport{Target: name, Namespace: ns, Kind: "Deployment"}

	dep, err := e.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		return e.explainPod(ctx, ns, name)
	}
	if err != nil {
		return rep, err
	}

	desired := int32(1)
	if dep.Spec.Replicas != nil {
		desired = *dep.Spec.Replicas
	}
	rep.Status = fmt.Sprintf("ready %d/%d, unavailable %d", dep.Status.ReadyReplicas, desired, dep.Status.UnavailableReplicas)
	rep.Chain = append(rep.Chain, ChainStep{
		Level:  "Deployment",
		Name:   name,
		Detail: rep.Status,
	})

	replicaSets, err := e.ownedReplicaSets(ctx, ns, dep)
	if err != nil {
		return rep, err
	}
	for _, rs := range replicaSets {
		rsDesired := int32(1)
		if rs.Spec.Replicas != nil {
			rsDesired = *rs.Spec.Replicas
		}
		rep.Chain = append(rep.Chain, ChainStep{
			Level:  "ReplicaSet",
			Name:   rs.Name,
			Detail: fmt.Sprintf("ready %d/%d", rs.Status.ReadyReplicas, rsDesired),
		})
		rep.Events = append(rep.Events, e.recentEvents(ctx, ns, "ReplicaSet", rs.Name)...)
	}

	pods, err := e.Client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(dep.Spec.Selector),
	})
	if err != nil {
		return rep, err
	}
	for _, pod := range pods.Items {
		rep.Chain = append(rep.Chain, ChainStep{
			Level:  "Pod",
			Name:   pod.Name,
			Detail: podChainDetail(pod),
		})
		rep.Findings = append(rep.Findings, diagnosePod(pod)...)
		rep.Events = append(rep.Events, e.recentEvents(ctx, ns, "Pod", pod.Name)...)
	}

	rep.Events = append(e.recentEvents(ctx, ns, "Deployment", name), rep.Events...)
	rep.Events = dedupeStrings(rep.Events)
	const maxEvents = 16
	if len(rep.Events) > maxEvents {
		rep.Events = rep.Events[:maxEvents]
	}

	if worst := worstPod(pods.Items); worst != nil && (problemScore(*worst) > 0 || dep.Status.ReadyReplicas < desired) {
		container := pickLogContainer(*worst)
		if tail, c, err := e.tailPodLogs(ctx, ns, worst.Name, container, explainLogTail); err == nil && strings.TrimSpace(tail) != "" {
			rep.LogPod = worst.Name
			rep.LogContainer = c
			rep.LogTail = tail
			rep.Chain = append(rep.Chain, ChainStep{
				Level:  "Logs",
				Name:   worst.Name,
				Detail: fmt.Sprintf("last %d lines (container=%s)", explainLogTail, firstNonEmpty(c, "(default)")),
			})
		}
	}

	rep.Findings = dedupeFindings(rep.Findings)
	rep.Summary = summarize(rep)
	return rep, nil
}

func (e *Explainer) ownedReplicaSets(ctx context.Context, ns string, dep *appsv1.Deployment) ([]appsv1.ReplicaSet, error) {
	list, err := e.Client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{
		LabelSelector: metav1.FormatLabelSelector(dep.Spec.Selector),
	})
	if err != nil {
		return nil, err
	}
	uid := dep.UID
	var owned []appsv1.ReplicaSet
	for _, rs := range list.Items {
		if replicaSetOwnedBy(&rs, uid) {
			owned = append(owned, rs)
		}
	}
	sort.Slice(owned, func(i, j int) bool {
		return owned[i].CreationTimestamp.After(owned[j].CreationTimestamp.Time)
	})
	return owned, nil
}

func replicaSetOwnedBy(rs *appsv1.ReplicaSet, depUID types.UID) bool {
	for _, ref := range rs.OwnerReferences {
		if ref.Kind == "Deployment" && ref.UID == depUID {
			return true
		}
	}
	return false
}

func podChainDetail(pod corev1.Pod) string {
	parts := []string{fmt.Sprintf("phase=%s", pod.Status.Phase)}
	for _, cs := range pod.Status.ContainerStatuses {
		if cs.State.Waiting != nil && cs.State.Waiting.Reason != "" {
			parts = append(parts, fmt.Sprintf("%s=%s", cs.Name, cs.State.Waiting.Reason))
		} else if !cs.Ready {
			parts = append(parts, fmt.Sprintf("%s=not-ready", cs.Name))
		}
	}
	return strings.Join(parts, ", ")
}

func worstPod(pods []corev1.Pod) *corev1.Pod {
	if len(pods) == 0 {
		return nil
	}
	sorted := append([]corev1.Pod(nil), pods...)
	sort.SliceStable(sorted, func(i, j int) bool {
		return problemScore(sorted[i]) > problemScore(sorted[j])
	})
	if problemScore(sorted[0]) == 0 {
		return &sorted[0]
	}
	return &sorted[0]
}

func problemScore(p corev1.Pod) int {
	score := 0
	switch p.Status.Phase {
	case corev1.PodFailed:
		score += 500
	case corev1.PodPending:
		score += 50
	}
	for _, cs := range p.Status.ContainerStatuses {
		if cs.State.Waiting != nil {
			switch cs.State.Waiting.Reason {
			case "CrashLoopBackOff":
				score += 1000
			case "ImagePullBackOff", "ErrImagePull", "CreateContainerConfigError":
				score += 800
			default:
				score += 200
			}
		}
		if !cs.Ready {
			score += 100
		}
		score += int(cs.RestartCount) * 10
		if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.ExitCode != 0 {
			score += 150
		}
	}
	for _, cond := range p.Status.Conditions {
		if cond.Type == corev1.PodScheduled && cond.Status == corev1.ConditionFalse {
			score += 300
		}
	}
	return score
}

func pickLogContainer(pod corev1.Pod) string {
	var worst string
	worstScore := -1
	for _, cs := range pod.Status.ContainerStatuses {
		s := 0
		if cs.State.Waiting != nil {
			s += 100
		}
		if !cs.Ready {
			s += 50
		}
		s += int(cs.RestartCount)
		if s > worstScore {
			worstScore = s
			worst = cs.Name
		}
	}
	if worst != "" {
		return worst
	}
	if len(pod.Spec.Containers) == 1 {
		return pod.Spec.Containers[0].Name
	}
	return ""
}

func (e *Explainer) tailPodLogs(ctx context.Context, ns, podName, container string, tail int64) (body string, usedContainer string, err error) {
	if tail <= 0 {
		tail = explainLogTail
	}
	opts := &corev1.PodLogOptions{TailLines: &tail}
	if container != "" {
		opts.Container = container
		usedContainer = container
	}
	stream, err := e.Client.CoreV1().Pods(ns).GetLogs(podName, opts).Stream(ctx)
	if err != nil {
		return "", container, err
	}
	defer stream.Close()
	raw, err := io.ReadAll(stream)
	if err != nil {
		return "", usedContainer, err
	}
	return string(raw), usedContainer, nil
}

func dedupeStrings(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
