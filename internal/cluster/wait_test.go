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

func TestWaitStatefulSetAlreadyReady(t *testing.T) {
	replicas := int32(2)
	client := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "default", Generation: 1},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           2,
			UpdatedReplicas:    2,
			ReadyReplicas:      2,
		},
	})
	var out bytes.Buffer
	err := (&Waiter{Client: client, Out: &out}).WaitStatefulSet(context.Background(), "default", "redis", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ready") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestWaitStatefulSetTimeout(t *testing.T) {
	replicas := int32(2)
	client := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "default", Generation: 2},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           1,
			UpdatedReplicas:    1,
			ReadyReplicas:      1,
		},
	})
	err := (&Waiter{Client: client}).WaitStatefulSet(context.Background(), "default", "redis", 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(err.Error(), "timed out waiting for StatefulSet/redis") {
		t.Fatalf("err=%v", err)
	}
}

func TestStatefulSetComplete(t *testing.T) {
	replicas := int32(1)
	sts := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 3},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 3,
			Replicas:           1,
			UpdatedReplicas:    1,
			ReadyReplicas:      1,
		},
	}
	if !statefulSetRolledOut(sts) {
		t.Fatal("expected complete")
	}
	sts.Status.ReadyReplicas = 0
	if statefulSetRolledOut(sts) {
		t.Fatal("expected incomplete")
	}
}

func TestWaitDaemonSetAlreadyReady(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "default", Generation: 1},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     1,
			DesiredNumberScheduled: 3,
			UpdatedNumberScheduled: 3,
			NumberReady:            3,
		},
	})
	var out bytes.Buffer
	err := (&Waiter{Client: client, Out: &out}).WaitDaemonSet(context.Background(), "default", "fluentd", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "ready") {
		t.Fatalf("output=%q", out.String())
	}
}

func TestWaitDaemonSetTimeout(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "fluent-log", Namespace: "default", Generation: 2},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     1,
			DesiredNumberScheduled: 3,
			UpdatedNumberScheduled: 1,
			NumberReady:            1,
		},
	})
	err := (&Waiter{Client: client}).WaitDaemonSet(context.Background(), "default", "fluent-log", 200*time.Millisecond)
	if err == nil {
		t.Fatal("expected timeout")
	}
	if !strings.Contains(err.Error(), "timed out waiting for DaemonSet/fluent-log") {
		t.Fatalf("err=%v", err)
	}
}

func TestDaemonSetReady(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 2},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     2,
			DesiredNumberScheduled: 5,
			UpdatedNumberScheduled: 5,
			NumberReady:            5,
		},
	}
	if !daemonSetReady(ds) {
		t.Fatal("expected ready")
	}
	ds.Status.NumberReady = 3
	if daemonSetReady(ds) {
		t.Fatal("expected not ready")
	}
}

func TestDaemonSetReadyDesiredZero(t *testing.T) {
	ds := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Generation: 1},
		Status: appsv1.DaemonSetStatus{
			ObservedGeneration:     1,
			DesiredNumberScheduled: 0,
			UpdatedNumberScheduled: 0,
			NumberReady:            0,
		},
	}
	if daemonSetReady(ds) {
		t.Fatal("expected not ready when desired=0")
	}
}
