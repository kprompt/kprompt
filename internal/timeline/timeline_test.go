package timeline

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestBuilderDeploymentTimeline(t *testing.T) {
	ns := "payments"
	now := time.Now().UTC()
	min := int32(1)
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api", Namespace: ns, UID: types.UID("dep1"),
				CreationTimestamp: metav1.NewTime(now.Add(-2 * time.Hour)),
			},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-rs1", Namespace: ns,
				CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Minute)),
				Annotations:       map[string]string{"deployment.kubernetes.io/revision": "2"},
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "dep1",
				}},
				Labels: map[string]string{"app": "api"},
			},
			Status: appsv1.ReplicaSetStatus{Replicas: 1, ReadyReplicas: 0},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-pod", Namespace: ns, Labels: map[string]string{"app": "api"},
			},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: ns},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod", Name: "api-pod", Namespace: ns,
			},
			Reason:        "BackOff",
			Message:       "Back-off restarting failed container",
			LastTimestamp: metav1.NewTime(now.Add(-5 * time.Minute)),
			Type:          "Warning",
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-hpa", Namespace: ns,
				CreationTimestamp: metav1.NewTime(now.Add(-40 * time.Minute)),
			},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment", Name: "api",
				},
				MinReplicas: &min,
				MaxReplicas: 5,
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 1, DesiredReplicas: 2,
			},
		},
	)

	doc, err := (&Builder{Client: client}).Run(context.Background(), Request{
		Name: "api", Namespace: ns, Kind: "Deployment",
		Prompt: "timeline for api", Window: time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Timeline) == 0 {
		t.Fatal("expected timeline entries")
	}
	if !hasCode(doc.Findings, "Timeline.Rollouts") {
		t.Fatalf("missing rollouts finding: %+v", doc.Findings)
	}
	if !hasCode(doc.Findings, "Timeline.HPA") {
		t.Fatalf("missing HPA finding: %+v", doc.Findings)
	}
	if !hasCode(doc.Findings, "Timeline.Events") {
		t.Fatalf("missing events finding: %+v", doc.Findings)
	}
	for i := 1; i < len(doc.Timeline); i++ {
		a, b := doc.Timeline[i-1].Timestamp, doc.Timeline[i].Timestamp
		if a != nil && b != nil && a.After(*b) {
			t.Fatalf("timeline not sorted at %d: %v > %v", i, a, b)
		}
	}
	for _, d := range []string{"prometheus", "otel", "mesh"} {
		found := false
		for _, x := range doc.Degraded {
			if x == d {
				found = true
			}
		}
		if !found {
			t.Fatalf("expected degraded %q", d)
		}
	}
}

func TestBuilderPodTimeline(t *testing.T) {
	ns := "payments"
	now := time.Now().UTC()
	client := fake.NewSimpleClientset(
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "ledger", Namespace: ns,
				CreationTimestamp: metav1.NewTime(now.Add(-10 * time.Minute)),
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "ev1", Namespace: ns},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod", Name: "ledger", Namespace: ns,
			},
			Reason:        "FailedScheduling",
			Message:       "persistentvolumeclaim not found",
			LastTimestamp: metav1.NewTime(now.Add(-2 * time.Minute)),
		},
	)
	doc, err := (&Builder{Client: client}).Run(context.Background(), Request{
		Name: "ledger", Namespace: ns, Kind: "Pod", Prompt: "timeline ledger",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Timeline) < 2 {
		t.Fatalf("expected create + event, got %d", len(doc.Timeline))
	}
}

func TestBuilderStatefulSetTimeline(t *testing.T) {
	ns := "payments"
	now := time.Now().UTC()
	client := fake.NewSimpleClientset(
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "db", Namespace: ns, UID: types.UID("sts1"),
				CreationTimestamp: metav1.NewTime(now.Add(-90 * time.Minute)),
			},
			Spec: appsv1.StatefulSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "db"}},
			},
		},
		&appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "db-rev-1", Namespace: ns,
				CreationTimestamp: metav1.NewTime(now.Add(-30 * time.Minute)),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "StatefulSet", Name: "db", UID: "sts1",
				}},
			},
			Revision: 3,
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "db-0", Namespace: ns, Labels: map[string]string{"app": "db"},
			},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "sts-ev", Namespace: ns},
			InvolvedObject: corev1.ObjectReference{
				Kind: "StatefulSet", Name: "db", Namespace: ns,
			},
			Reason:        "SuccessfulCreate",
			Message:       "create Pod db-0 in StatefulSet db successful",
			LastTimestamp: metav1.NewTime(now.Add(-20 * time.Minute)),
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "pod-ev", Namespace: ns},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod", Name: "db-0", Namespace: ns,
			},
			Reason:        "Pulled",
			Message:       "Container image pulled",
			LastTimestamp: metav1.NewTime(now.Add(-10 * time.Minute)),
		},
	)

	doc, err := (&Builder{Client: client}).Run(context.Background(), Request{
		Name: "db", Namespace: ns, Kind: "statefulset",
		Prompt: "timeline for statefulset db", Window: 2 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Target == nil || doc.Target.Kind != "StatefulSet" {
		t.Fatalf("expected StatefulSet target, got %+v", doc.Target)
	}
	if !hasCode(doc.Findings, "Timeline.ControllerRevisions") {
		t.Fatalf("missing controller revision finding: %+v", doc.Findings)
	}
	if !hasCode(doc.Findings, "Timeline.Events") {
		t.Fatalf("missing events finding: %+v", doc.Findings)
	}
	if !timelineContainsMessage(doc.Timeline, "ControllerRevision/db-rev-1 revision=3") {
		t.Fatalf("missing controller revision entry: %+v", doc.Timeline)
	}
	if !timelineContainsResource(doc.Timeline, "Pod", "db-0") {
		t.Fatalf("missing related pod evidence: %+v", doc.Timeline)
	}
}

func TestBuilderDaemonSetTimeline(t *testing.T) {
	ns := "payments"
	now := time.Now().UTC()
	client := fake.NewSimpleClientset(
		&appsv1.DaemonSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-agent", Namespace: ns, UID: types.UID("ds1"),
				CreationTimestamp: metav1.NewTime(now.Add(-70 * time.Minute)),
			},
			Spec: appsv1.DaemonSetSpec{
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "node-agent"}},
			},
		},
		&appsv1.ControllerRevision{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-agent-rev-1", Namespace: ns,
				CreationTimestamp: metav1.NewTime(now.Add(-40 * time.Minute)),
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "DaemonSet", Name: "node-agent", UID: "ds1",
				}},
			},
			Revision: 7,
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node-agent-abc", Namespace: ns, Labels: map[string]string{"app": "node-agent"},
			},
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "ds-ev", Namespace: ns},
			InvolvedObject: corev1.ObjectReference{
				Kind: "DaemonSet", Name: "node-agent", Namespace: ns,
			},
			Reason:        "RollingUpdate",
			Message:       "updated DaemonSet pods",
			LastTimestamp: metav1.NewTime(now.Add(-25 * time.Minute)),
		},
		&corev1.Event{
			ObjectMeta: metav1.ObjectMeta{Name: "ds-pod-ev", Namespace: ns},
			InvolvedObject: corev1.ObjectReference{
				Kind: "Pod", Name: "node-agent-abc", Namespace: ns,
			},
			Reason:        "Started",
			Message:       "Started container agent",
			LastTimestamp: metav1.NewTime(now.Add(-15 * time.Minute)),
		},
	)

	doc, err := (&Builder{Client: client}).Run(context.Background(), Request{
		Name: "node-agent", Namespace: ns, Kind: "ds",
		Prompt: "what happened to daemonset node-agent", Window: 2 * time.Hour,
	})
	if err != nil {
		t.Fatal(err)
	}
	if doc.Target == nil || doc.Target.Kind != "DaemonSet" {
		t.Fatalf("expected DaemonSet target, got %+v", doc.Target)
	}
	if !hasCode(doc.Findings, "Timeline.ControllerRevisions") {
		t.Fatalf("missing controller revision finding: %+v", doc.Findings)
	}
	if !hasCode(doc.Findings, "Timeline.Events") {
		t.Fatalf("missing events finding: %+v", doc.Findings)
	}
	if !timelineContainsMessage(doc.Timeline, "ControllerRevision/node-agent-rev-1 revision=7") {
		t.Fatalf("missing daemonset controller revision entry: %+v", doc.Timeline)
	}
	if !timelineContainsResource(doc.Timeline, "Pod", "node-agent-abc") {
		t.Fatalf("missing daemonset pod evidence: %+v", doc.Timeline)
	}
}

func hasCode(fs []incident.Finding, code string) bool {
	for _, f := range fs {
		if f.Code == code {
			return true
		}
	}
	return false
}

func timelineContainsMessage(entries []incident.EvidenceRef, want string) bool {
	for _, e := range entries {
		if strings.Contains(e.Message, want) {
			return true
		}
	}
	return false
}

func timelineContainsResource(entries []incident.EvidenceRef, kind, name string) bool {
	for _, e := range entries {
		if e.Resource != nil && e.Resource.Kind == kind && e.Resource.Name == name {
			return true
		}
	}
	return false
}
