package cluster

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDescribeDeployment(t *testing.T) {
	replicas := int32(2)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx:1.27-alpine"}},
				},
			},
		},
		Status: appsv1.DeploymentStatus{ReadyReplicas: 2, UpdatedReplicas: 2, AvailableReplicas: 2},
	})
	rep, err := (&Describer{Client: client}).Describe(context.Background(), DescribeRequest{
		Name: "api", Namespace: "demo", Kind: "Deployment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Kind != "Deployment" || !strings.Contains(rep.Status, "2/2") {
		t.Fatalf("%+v", rep)
	}
	joined := strings.Join(rep.Lines, "\n")
	if !strings.Contains(joined, "nginx:1.27-alpine") {
		t.Fatalf("lines=%v", rep.Lines)
	}
}

func TestDescribePod(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "api-0", Namespace: "demo"},
		Spec: corev1.PodSpec{
			NodeName: "node-1",
			Containers: []corev1.Container{{
				Name:  "app",
				Image: "nginx:1.27-alpine",
			}},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			PodIP: "10.0.0.1",
			ContainerStatuses: []corev1.ContainerStatus{{
				Name: "app", Ready: true, RestartCount: 1,
			}},
		},
	})
	rep, err := (&Describer{Client: client}).Describe(context.Background(), DescribeRequest{
		Name: "api-0", Namespace: "demo", Kind: "Pod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Status != "Running" {
		t.Fatalf("status=%s", rep.Status)
	}
}

func TestPickDeploymentPodPrefersRunning(t *testing.T) {
	labels := map[string]string{"app": "api"}
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
			Spec: appsv1.DeploymentSpec{
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
				},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-pending", Namespace: "demo", Labels: labels},
			Status:     corev1.PodStatus{Phase: corev1.PodPending},
			Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-ready", Namespace: "demo", Labels: labels},
			Status: corev1.PodStatus{
				Phase:            corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{Name: "app", Ready: true}},
			},
			Spec: corev1.PodSpec{Containers: []corev1.Container{{Name: "app", Image: "nginx"}}},
		},
	)
	pod, err := (&LogReader{Client: client}).pickDeploymentPod(context.Background(), "demo", "api")
	if err != nil {
		t.Fatal(err)
	}
	if pod.Name != "api-ready" {
		t.Fatalf("picked %s", pod.Name)
	}
}
