package suggest

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/optimize"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestFromOptimizeRightsizingBuildsPatchPlan(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "api",
						Image: "api:1",
						Resources: corev1.ResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceMemory: resource.MustParse("512Mi"),
							},
						},
					}},
				},
			},
		},
	})
	suggestions, err := FromOptimize(context.Background(), client, optimize.Report{
		Rightsizing: []optimize.RightsizingDelta{{
			Kind: optimize.WorkloadDeployment, Namespace: "prod", Name: "api",
			Resource: "memory", Field: "request",
			Current: "512Mi", Suggested: "256Mi", Direction: "lower",
			Message: "lower memory request 512Mi→256Mi",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 1 || suggestions[0].Plan == nil {
		t.Fatalf("%+v", suggestions)
	}
	if suggestions[0].Plan.Intent.Kind != "patch" || !suggestions[0].Plan.RequiresApproval {
		t.Fatalf("%+v", suggestions[0].Plan)
	}
	if suggestions[0].Plan.Actions[0].Op != planner.OpUpdate {
		t.Fatalf("op=%s", suggestions[0].Plan.Actions[0].Op)
	}
	if !strings.Contains(suggestions[0].Plan.Actions[0].Manifest, "256Mi") {
		t.Fatalf("manifest=%s", suggestions[0].Plan.Actions[0].Manifest)
	}
}

func TestFromOptimizeHPAScaleAndMaxedPrompt(t *testing.T) {
	des, cur := int32(4), int32(2)
	max := int32(5)
	suggestions, err := FromOptimize(context.Background(), fake.NewSimpleClientset(), optimize.Report{
		HPA: []optimize.HPAHint{
			{
				Kind: optimize.WorkloadDeployment, Namespace: "prod", Name: "api",
				HasHPA: true, Current: &cur, Desired: &des, Max: &max,
				Message: "desired ahead of current",
			},
			{
				Kind: optimize.WorkloadDeployment, Namespace: "prod", Name: "web",
				HasHPA: true, Maxed: true, HPAName: "web-hpa",
				Message: "at max",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	var scale, maxed *Suggestion
	for i := range suggestions {
		switch suggestions[i].Code {
		case "optimize.hpa.scale":
			scale = &suggestions[i]
		case "optimize.hpa.maxed":
			maxed = &suggestions[i]
		}
	}
	if scale == nil || scale.Plan == nil || scale.Plan.Actions[0].Replicas == nil || *scale.Plan.Actions[0].Replicas != 4 {
		t.Fatalf("scale=%+v", scale)
	}
	if maxed == nil || maxed.Plan != nil {
		t.Fatalf("maxed should be prompt-only: %+v", maxed)
	}
}

func TestFromOptimizeNilClient(t *testing.T) {
	out, err := FromOptimize(context.Background(), nil, optimize.Report{
		Rightsizing: []optimize.RightsizingDelta{{Name: "api", Kind: optimize.WorkloadDeployment}},
	})
	if err != nil || out != nil {
		t.Fatalf("out=%v err=%v", out, err)
	}
}
