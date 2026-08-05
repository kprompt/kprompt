package cleanup

import (
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestCleanupFindsUnusedAndStale(t *testing.T) {
	now := time.Date(2026, 7, 26, 12, 0, 0, 0, time.UTC)
	old := metav1.NewTime(now.Add(-48 * time.Hour))
	zero := int32(0)

	client := fake.NewSimpleClientset(
		// used ConfigMap referenced by the pod
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "app-config", Namespace: "payments"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "orphan-config", Namespace: "payments"}},
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "payments"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "used-secret", Namespace: "payments"}},
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "orphan-secret", Namespace: "payments"}},
		&corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "default-token-x", Namespace: "payments"},
			Type:       corev1.SecretTypeServiceAccountToken,
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
			Spec: corev1.PodSpec{
				Containers: []corev1.Container{{
					Name: "api",
					EnvFrom: []corev1.EnvFromSource{{
						ConfigMapRef: &corev1.ConfigMapEnvSource{
							LocalObjectReference: corev1.LocalObjectReference{Name: "app-config"},
						},
					}},
					Env: []corev1.EnvVar{{
						Name: "TOKEN",
						ValueFrom: &corev1.EnvVarSource{
							SecretKeyRef: &corev1.SecretKeySelector{
								LocalObjectReference: corev1.LocalObjectReference{Name: "used-secret"},
							},
						},
					}},
				}},
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "old-migrate", Namespace: "payments"},
			Status: batchv1.JobStatus{
				CompletionTime: &old,
				Conditions: []batchv1.JobCondition{{
					Type: batchv1.JobComplete, Status: corev1.ConditionTrue, LastTransitionTime: old,
				}},
			},
		},
		&batchv1.Job{
			ObjectMeta: metav1.ObjectMeta{Name: "recent-migrate", Namespace: "payments"},
			Status: batchv1.JobStatus{
				CompletionTime: ptrTime(now.Add(-1 * time.Hour)),
				Conditions: []batchv1.JobCondition{{
					Type: batchv1.JobComplete, Status: corev1.ConditionTrue,
				}},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "api-old", Namespace: "payments",
				OwnerReferences: []metav1.OwnerReference{{Kind: "Deployment", Name: "api"}},
			},
			Spec: appsv1.ReplicaSetSpec{Replicas: &zero},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-orphan", Namespace: "payments"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimPending},
		},
		&corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{Name: "pvc-bound", Namespace: "payments"},
			Status:     corev1.PersistentVolumeClaimStatus{Phase: corev1.ClaimBound},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-service", Namespace: "payments"},
			Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.1", Selector: map[string]string{"app": "nonexistent"}},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "empty-service", Namespace: "payments"},
			Subsets:    []corev1.EndpointSubset{},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "used-service", Namespace: "payments"},
			Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.2", Selector: map[string]string{"app": "api"}},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "used-service", Namespace: "payments"},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.244.0.5"}},
			}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "headless-service", Namespace: "payments"},
			Spec:       corev1.ServiceSpec{ClusterIP: corev1.ClusterIPNone, Selector: map[string]string{"app": "api"}},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "no-selector-service", Namespace: "payments"},
			Spec:       corev1.ServiceSpec{ClusterIP: "10.0.0.3"},
		},
	)

	got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{
		Namespace: "payments",
		Prompt:    "cleanup payments",
		Now:       now,
	})
	if err != nil {
		t.Fatal(err)
	}
	requireFinding(t, got, "Cleanup.UnusedConfigMap", "orphan-config")
	requireFinding(t, got, "Cleanup.UnusedSecret", "orphan-secret")
	requireFinding(t, got, "Cleanup.CompletedJob", "old-migrate")
	requireFinding(t, got, "Cleanup.OldReplicaSet", "api-old")
	requireFinding(t, got, "Cleanup.UnusedPVC", "pvc-orphan")
	requireFinding(t, got, "Cleanup.EmptyService", "empty-service")

	if hasFinding(got, "Cleanup.UnusedConfigMap", "app-config") {
		t.Fatal("used configmap flagged")
	}
	if hasFinding(got, "Cleanup.UnusedConfigMap", "kube-root-ca.crt") {
		t.Fatal("system configmap flagged")
	}
	if hasFinding(got, "Cleanup.UnusedSecret", "used-secret") {
		t.Fatal("used secret flagged")
	}
	if hasFinding(got, "Cleanup.UnusedSecret", "default-token-x") {
		t.Fatal("sa token secret flagged")
	}
	if hasFinding(got, "Cleanup.CompletedJob", "recent-migrate") {
		t.Fatal("recent job flagged")
	}
	if hasFinding(got, "Cleanup.UnusedPVC", "pvc-bound") {
		t.Fatal("bound PVC flagged")
	}
	if hasFinding(got, "Cleanup.EmptyService", "used-service") {
		t.Fatal("used service flagged")
	}
	if hasFinding(got, "Cleanup.EmptyService", "headless-service") {
		t.Fatal("headless service flagged")
	}
	if hasFinding(got, "Cleanup.EmptyService", "no-selector-service") {
		t.Fatal("no selector service flagged")
	}
	if !strings.Contains(got.Summary, "6 cleanup candidate(s)") {
		t.Fatalf("summary=%q", got.Summary)
	}
}

func TestCleanupCleanNamespace(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "kube-root-ca.crt", Namespace: "prod"}},
	)
	got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{
		Namespace: "prod", Prompt: "cleanup",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 0 {
		t.Fatalf("findings=%+v", got.Findings)
	}
	if !strings.Contains(got.Summary, "nothing to clean up") {
		t.Fatalf("summary=%q", got.Summary)
	}
}

func ptrTime(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)
	return &mt
}

func requireFinding(t *testing.T, got incident.Investigation, code, nameSubstr string) {
	t.Helper()
	if !hasFinding(got, code, nameSubstr) {
		t.Fatalf("missing finding code=%s name~%q in %+v", code, nameSubstr, got.Findings)
	}
}

func hasFinding(got incident.Investigation, code, nameSubstr string) bool {
	for _, f := range got.Findings {
		if f.Code == code && strings.Contains(f.Title, nameSubstr) {
			return true
		}
	}
	return false
}
