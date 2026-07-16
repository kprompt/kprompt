package cluster

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestExplainDeploymentChain(t *testing.T) {
	labels := map[string]string{"app": "api"}
	depUID := types.UID("dep-uid")
	replicas := int32(1)
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", UID: depUID},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 0, UnavailableReplicas: 1},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-rs",
				Namespace: "default",
				Labels:    labels,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1",
					Kind:       "Deployment",
					Name:       "api",
					UID:        depUID,
					Controller: boolPtr(true),
				}},
			},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
			},
			Status: appsv1.ReplicaSetStatus{ReadyReplicas: 0},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-pod", Namespace: "default", Labels: labels},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{Name: "app", Image: "bad:tag"}},
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "app",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff", Message: "pull failed"},
					},
				}},
			},
		},
	)

	rep, err := (&Explainer{Client: client}).Explain(context.Background(), ExplainRequest{
		Name: "api", Namespace: "default", Kind: "Deployment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Chain) < 3 {
		t.Fatalf("expected chain with deployment/rs/pod, got %+v", rep.Chain)
	}
	if rep.Chain[0].Level != "Deployment" || rep.Chain[1].Level != "ReplicaSet" || rep.Chain[2].Level != "Pod" {
		t.Fatalf("unexpected chain order: %+v", rep.Chain)
	}
	if !containsFindingCode(rep.Findings, "ImagePullBackOff") {
		t.Fatalf("expected ImagePullBackOff finding, got %+v", rep.Findings)
	}
}

func TestWorstPodPrefersCrashLoop(t *testing.T) {
	pods := []corev1.Pod{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "ok"},
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "bad"},
			Status: corev1.PodStatus{
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:  "app",
					State: corev1.ContainerState{Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"}},
				}},
			},
		},
	}
	w := worstPod(pods)
	if w == nil || w.Name != "bad" {
		t.Fatalf("worst=%v", w)
	}
}

func containsFindingCode(findings []Finding, code string) bool {
	for _, f := range findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func boolPtr(b bool) *bool { return &b }
