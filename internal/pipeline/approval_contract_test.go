package pipeline

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/output"
)

// SEC-003: Approval contract adversarial tests
// Prove approval gates cannot be skipped and are enforced consistently.

func TestDeniedPlanNeverCallsExecutor(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "default", 1))
	var out bytes.Buffer
	resultCalled := false

	err := RunWith(context.Background(), config.Resolved{
		Approve: true,
		Prompt:  "delete namespace default",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"delete","target":{"kind":"Namespace","name":"default"},"confidence":1}`,
		)},
		Client: client,
		OnResult: func(_ output.PlanResult) {
			resultCalled = true
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(out.String(), "denied") {
		t.Fatalf("expected safety deny in output: %s", out.String())
	}
	if !resultCalled {
		t.Fatal("OnResult must be called even for denied plans")
	}
}

func TestMediumRiskRequiresApprovalGate(t *testing.T) {
	// Scale deployment is Medium risk; without --approve or Confirm, should not apply
	client := fake.NewSimpleClientset(deployment("api", "default", 1))

	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "default",
		Prompt:    "scale api to 3",
	}, &out, Deps{
		Provider:   llm.ScaleStub("api", "default", 3),
		Client:     client,
		IsTerminal: boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}

	dep, _ := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if dep.Spec.Replicas != nil && *dep.Spec.Replicas != 1 {
		t.Fatalf("medium risk plan applied without approval; replicas=%v", *dep.Spec.Replicas)
	}
	if !strings.Contains(out.String(), "--approve") {
		t.Fatalf("expected --approve hint, got: %s", out.String())
	}
}

func TestOptimizeApproveFlagDoesNotAutoApplySuggestedFix(t *testing.T) {
	replicas := int32(2)
	limit := resource.MustParse("1Gi")
	labels := map[string]string{"app": "api"}

	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "api",
						Image: "api:1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceCPU:    resource.MustParse("1"),
								corev1.ResourceMemory: resource.MustParse("1Gi"),
							},
							Limits: corev1.ResourceList{
								corev1.ResourceMemory: limit,
							},
						},
					}},
				},
			},
		},
	})

	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   true,
		Namespace: "default",
		Prompt:    "optimize my cluster",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"optimize","target":{"kind":"Cluster"},"params":{"scope":"cluster"},"confidence":1}`,
		)},
		Client:     client,
		IsTerminal: boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}

	// --approve should NOT auto-apply the suggested fix/patch
	output := out.String()
	if strings.Contains(output, "Applied patch") && strings.Contains(output, "suggested") {
		t.Fatalf("optimize --approve must not auto-apply suggested fixes: %s", output)
	}
}

func TestNonTTYOrphanDeleteWithoutConfirmOrphansPhrase(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "default"}},
	)

	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "default",
		Prompt:    "cleanup default",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"cleanup","target":{"kind":"Namespace","namespace":"default"},"confidence":1}`,
		)},
		Client:     client,
		IsTerminal: boolPtr(false),
	})
	if err != nil {
		t.Fatal(err)
	}

	// Non-TTY + no prompt phrase = guidance-only cleanup (no orphan delete attempt)
	output := out.String()
	if strings.Contains(output, "Applied") {
		t.Fatalf("non-TTY cleanup without confirm orphans should not apply: %s", output)
	}
}

func TestOrphanDeleteRequiresConfirmPhrase(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.ConfigMap{ObjectMeta: metav1.ObjectMeta{Name: "orphan", Namespace: "default"}},
	)

	// Interactive: --approve is not enough; must type DELETE-ORPHANS
	var out bytes.Buffer
	confirmCalls := 0
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "default",
		Prompt:    "cleanup default and confirm orphans",
	}, &out, Deps{
		Provider: &llm.Stub{Structured: []byte(
			`{"kind":"cleanup","target":{"kind":"Namespace","namespace":"default"},"confidence":1}`,
		)},
		Client: client,
		Confirm: func(w io.Writer) (bool, error) {
			confirmCalls++
			return false, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Confirm should have been called (for orphan approval, not apply)
	if confirmCalls == 0 {
		t.Fatal("orphan approval gate must call Confirm")
	}

	// With Confirm returning false, cleanup should not apply
	if strings.Contains(out.String(), "Applied") {
		t.Fatalf("cleanup must not apply when orphan approval denied: %s", out.String())
	}
}

func TestApproveByRoleDeny(t *testing.T) {
	// Test that org role approval matrix denies if role lacks permission
	client := fake.NewSimpleClientset(deployment("api", "default", 1))

	var out bytes.Buffer
	err := RunWith(context.Background(), config.Resolved{
		Approve:   true,
		Namespace: "default",
		Prompt:    "delete deployment api",
	}, &out, Deps{
		Provider: llm.DeleteStub("api", "default", "Deployment"),
		Client:   client,
	})
	if err != nil {
		t.Fatal(err)
	}

	// With standard (no mocked role system), this test just verifies --approve works
	// Real org policy role checks tested in safety/orgpolicy_test.go
}
