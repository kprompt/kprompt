package optimize

import (
	"context"
	"strings"
	"testing"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectHPAHintsMaxedAndStatic(t *testing.T) {
	max := int32(5)
	client := fake.NewSimpleClientset(
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "prod"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment",
					Name: "api",
				},
				MaxReplicas: max,
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{
				CurrentReplicas: 5,
				DesiredReplicas: 5,
			},
		},
	)

	result := CollectHPAHints(context.Background(), client, nil, []Workload{
		{Kind: WorkloadDeployment, Namespace: "prod", Name: "api", Replicas: 5},
		{Kind: WorkloadDeployment, Namespace: "prod", Name: "web", Replicas: 3},
	}, "prod")

	if result.Skipped || result.WithHPA != 1 || result.Maxed != 1 || result.Static != 1 {
		t.Fatalf("%+v", result)
	}
	var foundMaxed, foundStatic bool
	for _, h := range result.Hints {
		if h.Name == "api" && h.Maxed && strings.Contains(h.Message, "at max") {
			foundMaxed = true
		}
		if h.Name == "web" && h.StaticReplicas && strings.Contains(h.Message, "static replicas") {
			foundStatic = true
		}
	}
	if !foundMaxed || !foundStatic {
		t.Fatalf("hints=%+v", result.Hints)
	}

	rep := BuildScaffold(Request{Namespace: "prod"})
	ApplyHPA(&rep, result)
	if rep.Sections.HPA.Status != SectionReady {
		t.Fatalf("%+v", rep.Sections.HPA)
	}
	if len(rep.HPA) != 2 {
		t.Fatalf("hpa=%+v", rep.HPA)
	}
	if len(rep.Suggestions) == 0 {
		t.Fatal("expected suggestions for maxed/static")
	}
}

func TestCollectHPAHintsRequiresClient(t *testing.T) {
	result := CollectHPAHints(context.Background(), nil, nil, []Workload{{Name: "api"}}, "")
	if !result.Skipped {
		t.Fatalf("%+v", result)
	}
}

func TestCollectHPAHintsNoHintForSingleReplica(t *testing.T) {
	client := fake.NewSimpleClientset()
	result := CollectHPAHints(context.Background(), client, nil, []Workload{
		{Kind: WorkloadDeployment, Namespace: "default", Name: "api", Replicas: 1},
	}, "default")
	if result.Skipped || len(result.Hints) != 0 || result.Static != 0 {
		t.Fatalf("%+v", result)
	}
	rep := BuildScaffold(Request{})
	ApplyHPA(&rep, result)
	if rep.Sections.HPA.Status != SectionReady {
		t.Fatalf("%+v", rep.Sections.HPA)
	}
}
