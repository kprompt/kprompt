package verify

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/planner"
)

// --- Deployment Tests ---

func TestPlanScaleReady(t *testing.T) {
	rep := int32(3)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &rep},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           3,
			UpdatedReplicas:    3,
			AvailableReplicas:  3,
		},
	})
	want := int32(3)
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:       planner.OpScale,
			Object:   planner.ObjectRef{Kind: "Deployment", Name: "api", Namespace: "demo"},
			Replicas: &want,
		}},
	})
	if report.Status != OK {
		t.Fatalf("expected OK, got: %+v", report)
	}
}

func TestPlanScalePending(t *testing.T) {
	rep := int32(3)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo", Generation: 2},
		Spec:       appsv1.DeploymentSpec{Replicas: &rep},
		Status: appsv1.DeploymentStatus{
			ObservedGeneration: 2,
			Replicas:           1,
			UpdatedReplicas:    1,
			AvailableReplicas:  1,
		},
	})
	want := int32(3)
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:       planner.OpScale,
			Object:   planner.ObjectRef{Kind: "Deployment", Name: "api", Namespace: "demo"},
			Replicas: &want,
		}},
	})
	if report.Status != Pending {
		t.Fatalf("expected Pending, got: %+v", report)
	}
}

// --- StatefulSet Tests ---

func TestStatefulSetReady(t *testing.T) {
	rep := int32(3)
	client := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "demo"},
		Spec:       appsv1.StatefulSetSpec{Replicas: &rep},
		Status: appsv1.StatefulSetStatus{
			Replicas:        3,
			ReadyReplicas:   3,
			UpdatedReplicas: 3,
		},
	})
	want := int32(3)
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:       planner.OpScale,
			Object:   planner.ObjectRef{Kind: "StatefulSet", Name: "db", Namespace: "demo"},
			Replicas: &want,
		}},
	})
	if report.Status != OK {
		t.Fatalf("expected OK, got: %+v", report)
	}
}

func TestStatefulSetPending(t *testing.T) {
	rep := int32(3)
	client := fake.NewSimpleClientset(&appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "demo"},
		Spec:       appsv1.StatefulSetSpec{Replicas: &rep},
		Status: appsv1.StatefulSetStatus{
			Replicas:        3,
			ReadyReplicas:   1,
			UpdatedReplicas: 1,
		},
	})
	want := int32(3)
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:       planner.OpUpdate,
			Object:   planner.ObjectRef{Kind: "StatefulSet", Name: "db", Namespace: "demo"},
			Replicas: &want,
		}},
	})
	if report.Status != Pending {
		t.Fatalf("expected Pending, got: %+v", report)
	}
}

// --- DaemonSet Tests ---

func TestDaemonSetReady(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "kube-system"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			NumberReady:            5,
			UpdatedNumberScheduled: 5,
		},
	})
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpUpdate,
			Object: planner.ObjectRef{Kind: "DaemonSet", Name: "fluentd", Namespace: "kube-system"},
		}},
	})
	if report.Status != OK {
		t.Fatalf("expected OK, got: %+v", report)
	}
}

func TestDaemonSetPending(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{Name: "fluentd", Namespace: "kube-system"},
		Status: appsv1.DaemonSetStatus{
			DesiredNumberScheduled: 5,
			NumberReady:            2,
			UpdatedNumberScheduled: 2,
		},
	})
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpUpdate,
			Object: planner.ObjectRef{Kind: "DaemonSet", Name: "fluentd", Namespace: "kube-system"},
		}},
	})
	if report.Status != Pending {
		t.Fatalf("expected Pending, got: %+v", report)
	}
}

// --- Helm Release Tests ---

func TestHelmReleasePodsReady(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "redis-master-0",
			Namespace: "default",
			Labels:    map[string]string{"app.kubernetes.io/instance": "redis"},
		},
		Status: corev1.PodStatus{
			Phase: corev1.PodRunning,
			Conditions: []corev1.PodCondition{
				{Type: corev1.PodReady, Status: corev1.ConditionTrue},
			},
		},
	})
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpHelmInstall,
			Object: planner.ObjectRef{Kind: "HelmRelease", Name: "redis", Namespace: "default"},
		}},
	})
	if report.Status != OK {
		t.Fatalf("expected OK, got: %+v", report)
	}
}

func TestHelmReleaseControllerFallback(t *testing.T) {
	rep := int32(1)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "nginx-ingress-controller",
			Namespace: "default",
			Labels:    map[string]string{"app.kubernetes.io/instance": "nginx-ingress"},
		},
		Spec: appsv1.DeploymentSpec{Replicas: &rep},
		Status: appsv1.DeploymentStatus{
			Replicas:          1,
			UpdatedReplicas:   1,
			AvailableReplicas: 1,
		},
	})
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpHelmUpgrade,
			Object: planner.ObjectRef{Kind: "HelmRelease", Name: "nginx-ingress", Namespace: "default"},
		}},
	})
	if report.Status != OK {
		t.Fatalf("expected OK, got: %+v", report)
	}
}

func TestHelmReleaseSkippedWhenNoWorkloads(t *testing.T) {
	client := fake.NewSimpleClientset() // Empty cluster
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpHelmInstall,
			Object: planner.ObjectRef{Kind: "HelmRelease", Name: "my-config-chart", Namespace: "default"},
		}},
	})
	if report.Checks[0].Status != Skipped {
		t.Fatalf("expected Skipped check status, got: %+v", report)
	}
}

// --- Deletion Verification Tests ---

func TestPlanDeleteGone(t *testing.T) {
	client := fake.NewSimpleClientset()
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpDelete,
			Object: planner.ObjectRef{Kind: "Pod", Name: "x", Namespace: "demo"},
		}},
	})
	if report.Status != OK {
		t.Fatalf("expected OK, got: %+v", report)
	}
}

func TestPlanDeleteStillPresent(t *testing.T) {
	client := fake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "demo"},
	})
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpDelete,
			Object: planner.ObjectRef{Kind: "Pod", Name: "x", Namespace: "demo"},
		}},
	})
	if report.Status != Failed {
		t.Fatalf("expected Failed, got: %+v", report)
	}
}

func TestPlanDeleteStatefulSetGone(t *testing.T) {
	client := fake.NewSimpleClientset()
	report := Plan(context.Background(), client, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{{
			Op:     planner.OpDelete,
			Object: planner.ObjectRef{Kind: "StatefulSet", Name: "redis", Namespace: "demo"},
		}},
	})
	if report.Status != OK {
		t.Fatalf("expected OK, got: %+v", report)
	}
}

func TestPlanDeleteJobAndReplicaSet(t *testing.T) {
	// 1. Verify Job and ReplicaSet deleted successfully (NotFound)
	clientEmpty := fake.NewSimpleClientset()
	report1 := Plan(context.Background(), clientEmpty, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "Job", Name: "old-migrate", Namespace: "demo"}},
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "ReplicaSet", Name: "api-old", Namespace: "demo"}},
		},
	})
	if report1.Status != OK {
		t.Fatalf("expected OK when resources are gone, got: %+v", report1)
	}

	// 2. Verify deletion fails if still present
	clientPresent := fake.NewSimpleClientset(
		&batchv1.Job{ObjectMeta: metav1.ObjectMeta{Name: "old-migrate", Namespace: "demo"}},
		&appsv1.ReplicaSet{ObjectMeta: metav1.ObjectMeta{Name: "api-old", Namespace: "demo"}},
	)
	report2 := Plan(context.Background(), clientPresent, planner.ExecutionPlan{
		RequiresApproval: true,
		Actions: []planner.Action{
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "Job", Name: "old-migrate", Namespace: "demo"}},
			{Op: planner.OpDelete, Object: planner.ObjectRef{Kind: "ReplicaSet", Name: "api-old", Namespace: "demo"}},
		},
	})
	if report2.Status != Failed {
		t.Fatalf("expected Failed when resources are still present, got: %+v", report2)
	}
}
