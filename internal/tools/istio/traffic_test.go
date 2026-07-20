package istio

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	metafake "k8s.io/client-go/testing"
)

func TestSummarizeVirtualServiceCanary(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "networking.istio.io/v1beta1",
		"kind":       "VirtualService",
		"metadata":   map[string]any{"name": "payments", "namespace": "prod"},
		"spec": map[string]any{
			"hosts": []any{"payments"},
			"http": []any{
				map[string]any{
					"route": []any{
						map[string]any{
							"destination": map[string]any{"host": "payments", "subset": "v1"},
							"weight":      int64(90),
						},
						map[string]any{
							"destination": map[string]any{"host": "payments", "subset": "v2"},
							"weight":      int64(10),
						},
					},
				},
			},
		},
	}}
	sum := summarizeVirtualService(obj)
	if !sum.Canary || len(sum.Routes) != 1 || len(sum.Routes[0].Splits) != 2 {
		t.Fatalf("%+v", sum)
	}
	if sum.Routes[0].Splits[0].Weight != 90 || sum.Routes[0].Splits[1].Subset != "v2" {
		t.Fatalf("%+v", sum.Routes[0].Splits)
	}
}

func TestSummarizeTrafficWithClient(t *testing.T) {
	gvr := VirtualServiceGVR
	scheme := runtime.NewScheme()
	client := fake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{gvr: "VirtualServiceList"},
	)
	client.PrependReactor("list", "virtualservices", func(action metafake.Action) (bool, runtime.Object, error) {
		list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{
			Object: map[string]any{
				"apiVersion": "networking.istio.io/v1beta1",
				"kind":       "VirtualService",
				"metadata":   map[string]any{"name": "api", "namespace": "default"},
				"spec": map[string]any{
					"hosts": []any{"api"},
					"http": []any{
						map[string]any{
							"route": []any{
								map[string]any{
									"destination": map[string]any{"host": "api"},
									"weight":      int64(100),
								},
							},
						},
					},
				},
			},
		}}}
		list.SetGroupVersionKind(schema.GroupVersionKind{Group: gvr.Group, Version: gvr.Version, Kind: "VirtualServiceList"})
		return true, list, nil
	})

	rep, err := SummarizeTrafficWithClient(t.Context(), client, TrafficRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.VirtualServices) != 1 || !strings.Contains(rep.Summary, "1 VirtualService") {
		t.Fatalf("%+v", rep)
	}
	if rep.VirtualServices[0].Canary {
		t.Fatal("single destination should not be canary")
	}
}
