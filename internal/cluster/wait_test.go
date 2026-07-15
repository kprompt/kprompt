package cluster

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestWaitDeploymentAlreadyReady(t *testing.T) {
	replicas := int32(2)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", Generation: 1},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			UpdatedReplicas:    2,
			AvailableReplicas:  2,
		},
	})
	var out bytes.Buffer
	err := (&Waiter{Client: client, Out: &out}).WaitDeployment(context.Background(), "default", "api", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ready") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestWaitDeploymentTimeout(t *testing.T) {
	replicas := int32(2)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default", Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 1,
			Replicas:           1,
			UpdatedReplicas:    1,
			AvailableReplicas:  1,
		},
	})
	err := (&Waiter{Client: client}).WaitDeployment(context.Background(), "default", "api", 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(err.Error(), "timed out waiting for Deployment/api") {
		t.Fatalf("err=%v", err)
	}
}

func TestDeploymentComplete(t *testing.T) {
	replicas := int32(1)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Generation: 3},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 3,
			Replicas:           1,
			UpdatedReplicas:    1,
			AvailableReplicas:  1,
		},
	}
	if !deploymentComplete(dep) {
		t.Fatal("expected complete")
	}
	dep.Status.AvailableReplicas = 0
	if deploymentComplete(dep) {
		t.Fatal("expected incomplete")
	}
}
