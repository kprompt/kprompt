package planner

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
	"sigs.k8s.io/yaml"

	"github.com/kprompt/kprompt/internal/intent"
)

func TestEnrichScaleDiff(t *testing.T) {
	replicas := int32(1)
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "default"},
		Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
	})
	want := int32(5)
	plan := ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindScale},
		Actions: []Action{{
			Op:       OpScale,
			Object:   ObjectRef{Kind: "Deployment", Name: "api", Namespace: "default"},
			Replicas: &want,
			Diff:     "scale placeholder",
		}},
	}
	EnrichDiffs(context.Background(), client, &plan)
	if plan.Actions[0].Diff != "replicas: 1 → 5" {
		t.Fatalf("diff=%q", plan.Actions[0].Diff)
	}
}

func TestEnrichDeployCreateVsUpdate(t *testing.T) {
	desired := &appsv1.Deployment{
		TypeMeta:   metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "demo"},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "redis", Image: "redis:7-alpine"}},
				},
			},
		},
	}
	raw, _ := yaml.Marshal(desired)

	createPlan := ExecutionPlan{
		Actions: []Action{{
			Op:       OpCreate,
			Object:   ObjectRef{Kind: "Deployment", Name: "redis", Namespace: "demo"},
			Manifest: string(raw),
		}},
	}
	EnrichDiffs(context.Background(), fake.NewSimpleClientset(), &createPlan)
	if createPlan.Actions[0].Op != OpCreate {
		t.Fatalf("op=%s", createPlan.Actions[0].Op)
	}
	if !strings.Contains(createPlan.Actions[0].Diff, "(create)") {
		t.Fatalf("diff=%q", createPlan.Actions[0].Diff)
	}
	if !strings.Contains(createPlan.Actions[0].Diff, "redis:7-alpine") {
		t.Fatalf("diff=%q", createPlan.Actions[0].Diff)
	}

	existing := desired.DeepCopy()
	existing.Spec.Replicas = int32Ptr(2)
	existing.Spec.Template.Spec.Containers[0].Image = "redis:6-alpine"
	updatePlan := ExecutionPlan{
		Actions: []Action{{
			Op:       OpCreate,
			Object:   ObjectRef{Kind: "Deployment", Name: "redis", Namespace: "demo"},
			Manifest: string(raw),
		}},
	}
	EnrichDiffs(context.Background(), fake.NewSimpleClientset(existing), &updatePlan)
	if updatePlan.Actions[0].Op != OpUpdate {
		t.Fatalf("op=%s", updatePlan.Actions[0].Op)
	}
	diff := updatePlan.Actions[0].Diff
	if !strings.Contains(diff, "(update)") || !strings.Contains(diff, "redis:6-alpine → redis:7-alpine") {
		t.Fatalf("diff=%q", diff)
	}
	if !strings.Contains(diff, "replicas: 2 → 1") {
		t.Fatalf("diff=%q", diff)
	}
}

func int32Ptr(v int32) *int32 { return &v }
