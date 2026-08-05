package executor

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestDeleteDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "demo"},
	})
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op: planner.OpDelete,
			Object: planner.ObjectRef{
				Kind: "Deployment", Name: "redis", Namespace: "demo",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AppsV1().Deployments("demo").Get(context.Background(), "redis", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected deployment deleted")
	}
}

func TestApplyStatefulSetAndDaemonSet(t *testing.T) {
	client := fake.NewSimpleClientset(
		&appsv1.StatefulSet{ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "demo"}},
		&appsv1.DaemonSet{ObjectMeta: metav1.ObjectMeta{Name: "agent", Namespace: "demo"}},
	)
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{
			{
				Op:       planner.OpUpdate,
				Object:   planner.ObjectRef{Kind: "StatefulSet", Name: "db", Namespace: "demo"},
				Manifest: "apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: db\n  namespace: demo\n  labels:\n    hardened: \"true\"\n",
			},
			{
				Op:       planner.OpUpdate,
				Object:   planner.ObjectRef{Kind: "DaemonSet", Name: "agent", Namespace: "demo"},
				Manifest: "apiVersion: apps/v1\nkind: DaemonSet\nmetadata:\n  name: agent\n  namespace: demo\n  labels:\n    hardened: \"true\"\n",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	sts, err := client.AppsV1().StatefulSets("demo").Get(context.Background(), "db", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if sts.Labels["hardened"] != "true" {
		t.Fatalf("StatefulSet not updated: %+v", sts.Labels)
	}
	ds, err := client.AppsV1().DaemonSets("demo").Get(context.Background(), "agent", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if ds.Labels["hardened"] != "true" {
		t.Fatalf("DaemonSet not updated: %+v", ds.Labels)
	}
}

func TestScaleStatefulSet(t *testing.T) {
	replicas := int32(1)
	client := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "demo"},
		Spec:       appsv1.StatefulSetSpec{Replicas: &replicas},
	})
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op: planner.OpScale,
			Object: planner.ObjectRef{
				Kind:      "StatefulSet",
				Name:      "redis",
				Namespace: "demo",
			},
			Replicas: int32Ptr(3),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	sts, err := client.AppsV1().StatefulSets("demo").Get(context.Background(), "redis", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if sts.Spec.Replicas == nil || *sts.Spec.Replicas != 3 {
		t.Fatalf("replicas=%v", sts.Spec.Replicas)
	}
}

func TestDeleteJobAndReplicaSet(t *testing.T) {
	client := fake.NewSimpleClientset(
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "old-migrate", Namespace: "payments"}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-old", Namespace: "payments"}},
	)
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "Job", Name: "old-migrate", Namespace: "payments"}},
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "ReplicaSet", Name: "api-old", Namespace: "payments"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.BatchV1().Jobs("payments").Get(context.Background(), "old-migrate", metav1.GetOptions{}); err == nil {
		t.Fatal("expected Job deleted")
	}
	if _, err := client.AppsV1().ReplicaSets("payments").Get(context.Background(), "api-old", metav1.GetOptions{}); err == nil {
		t.Fatal("expected ReplicaSet deleted")
	}
}

func TestDeleteConfigMapAndSecret(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "orphan-config", Namespace: "payments"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "orphan-secret", Namespace: "payments"}},
	)
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "ConfigMap", Name: "orphan-config", Namespace: "payments"}},
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "Secret", Name: "orphan-secret", Namespace: "payments"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.CoreV1().ConfigMaps("payments").Get(context.Background(), "orphan-config", metav1.GetOptions{}); err == nil {
		t.Fatal("expected ConfigMap deleted")
	}
	if _, err := client.CoreV1().Secrets("payments").Get(context.Background(), "orphan-secret", metav1.GetOptions{}); err == nil {
		t.Fatal("expected Secret deleted")
	}
}

func TestDeleteUnknownKindRejected(t *testing.T) {
	client := fake.NewSimpleClientset()
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op:     planner.OpDelete,
			Object: planner.ObjectRef{Kind: "CronTab", Name: "nightly", Namespace: "default"},
		}},
	})
	if err == nil {
		t.Fatal("expected unknown delete kind to fail")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func int32Ptr(v int32) *int32 { return &v }
