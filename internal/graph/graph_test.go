package graph

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func int32PtrLocal(v int32) *int32 { return &v }

func TestBuildServiceGraphRoutesAndNetworkPolicy(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "api"},
			},
		},
		&discoveryv1.EndpointSlice{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-abc",
				Namespace: "prod",
				Labels:    map[string]string{discoveryv1.LabelServiceName: "api"},
			},
			Ports: []discoveryv1.EndpointPort{{Port: int32PtrLocal(8080)}},
			Endpoints: []discoveryv1.Endpoint{{
				TargetRef: &corev1.ObjectReference{Kind: "Pod", Name: "api-1", Namespace: "prod"},
			}},
		},
		&networkingv1.NetworkPolicy{
			ObjectMeta: metav1.ObjectMeta{Name: "allow-api", Namespace: "prod"},
			Spec: networkingv1.NetworkPolicySpec{
				PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			},
		},
	)

	rep, err := Build(context.Background(), client, Request{
		Namespace:            "prod",
		IncludeNetworkPolicy: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Scope != ScopeNamespace || rep.Type != "service-graph" {
		t.Fatalf("%+v", rep)
	}
	if len(rep.Nodes) < 3 {
		t.Fatalf("nodes=%+v", rep.Nodes)
	}
	var hasRoute, hasSelects bool
	for _, e := range rep.Edges {
		if e.Type == EdgeRoutes && e.Source == SourceKubernetes {
			hasRoute = true
		}
		if e.Type == EdgeSelects {
			hasSelects = true
		}
	}
	if !hasRoute || !hasSelects {
		t.Fatalf("edges=%+v", rep.Edges)
	}
}

func TestBuildRequiresClient(t *testing.T) {
	_, err := Build(context.Background(), nil, Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}
