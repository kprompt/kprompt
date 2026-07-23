package planner

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"
)

func TestEnrichBlastRadiusDeployment(t *testing.T) {
	rep := int32(2)
	uid := types.UID("dep-uid")
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api",
				Namespace: "demo",
				UID:       uid,
				Labels:    map[string]string{"app": "api", "tier": "front"},
			},
			Spec: appsv1.DeploymentSpec{
				Replicas: &rep,
				Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
				},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-abc",
				Namespace: "demo",
				OwnerReferences: []metav1.OwnerReference{{
					Kind: "Deployment", Name: "api", UID: uid,
				}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
			Spec:       corev1.ServiceSpec{Selector: map[string]string{"app": "api"}},
		},
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "api-hpa", Namespace: "demo"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
					Kind: "Deployment", Name: "api",
				},
				MaxReplicas: 10,
			},
		},
		&networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "api-netpol", Namespace: "demo"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			},
		},
	)

	scaleTo := int32(5)
	plan := ExecutionPlan{
		RequiresApproval: true,
		Actions: []Action{{
			Op:       OpScale,
			Object:   ObjectRef{Kind: "Deployment", Name: "api", Namespace: "demo"},
			Replicas: &scaleTo,
		}},
	}
	EnrichBlastRadius(context.Background(), client, &plan)
	if plan.BlastRadius == nil {
		t.Fatal("expected blast radius")
	}
	br := plan.BlastRadius
	if len(br.Namespaces) != 1 || br.Namespaces[0] != "demo" {
		t.Fatalf("namespaces=%v", br.Namespaces)
	}
	if len(br.Targets) != 1 {
		t.Fatalf("targets=%d", len(br.Targets))
	}
	target := br.Targets[0]
	if target.Labels["app"] != "api" {
		t.Fatalf("labels=%v", target.Labels)
	}
	got := map[string]string{}
	for _, r := range target.Related {
		got[r.Kind+"/"+r.Name] = r.Relation
	}
	want := map[string]string{
		"HorizontalPodAutoscaler/api-hpa": "scales",
		"Service/api":                     "routes-to",
		"NetworkPolicy/api-netpol":        "selects-pods",
		"ReplicaSet/api-abc":              "owned",
	}
	for k, rel := range want {
		if got[k] != rel {
			t.Fatalf("related missing %s=%s got=%v", k, rel, got)
		}
	}
}

func TestEnrichBlastRadiusSkippedWhenNoApproval(t *testing.T) {
	plan := ExecutionPlan{
		RequiresApproval: false,
		Actions: []Action{{
			Op:     OpScale,
			Object: ObjectRef{Kind: "Deployment", Name: "api", Namespace: "demo"},
		}},
	}
	EnrichBlastRadius(context.Background(), fake.NewSimpleClientset(), &plan)
	if plan.BlastRadius != nil {
		t.Fatalf("unexpected %+v", plan.BlastRadius)
	}
}
