package impact

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	metafake "k8s.io/client-go/testing"

	toolistio "github.com/kprompt/kprompt/internal/tools/istio"
)

func TestServiceImpactVirtualService(t *testing.T) {
	client := kubefake.NewSimpleClientset(
		service("redis", map[string]string{"app": "redis"}),
		deployment("redis", map[string]string{"app": "redis"}, nil),
	)
	gvr := toolistio.VirtualServiceGVR
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "VirtualServiceList"},
	)
	dyn.PrependReactor("list", "virtualservices", func(action metafake.Action) (bool, runtime.Object, error) {
		list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{
			Object: map[string]any{
				"apiVersion": "networking.istio.io/v1beta1",
				"kind":       "VirtualService",
				"metadata":   map[string]any{"name": "redis-vs", "namespace": "payments"},
				"spec": map[string]any{
					"hosts": []any{"redis"},
					"http": []any{
						map[string]any{
							"route": []any{
								map[string]any{
									"destination": map[string]any{"host": "redis.payments.svc"},
									"weight":      int64(100),
								},
							},
						},
					},
				},
			},
		}}}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group: gvr.Group, Version: gvr.Version, Kind: "VirtualServiceList",
		})
		return true, list, nil
	})

	got, err := (&Analyzer{Client: client, Dynamic: dyn}).Run(context.Background(), Request{
		Name: "redis", Namespace: "payments", Kind: "Service",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireFinding(t, got, "Impact.VirtualService", "VirtualService/redis-vs routes to Service/redis")
	if !strings.Contains(got.Summary, "1 VirtualService route(s)") {
		t.Fatalf("summary=%q", got.Summary)
	}
	for _, d := range got.Degraded {
		if d == "mesh" {
			t.Fatalf("mesh should not be degraded: %v", got.Degraded)
		}
	}
	if !contains(got.Degraded, "otel") {
		t.Fatalf("otel still expected: %v", got.Degraded)
	}
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
