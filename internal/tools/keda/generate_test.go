package keda

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGenerateScaledObject(t *testing.T) {
	manifest, summary, err := GenerateScaledObject(ScaledObjectRequest{
		Name: "api-keda", Namespace: "default", TargetName: "api",
		MinReplicas: 0, MaxReplicas: 10, Trigger: "cpu",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifest, "kind: ScaledObject") || !strings.Contains(manifest, "keda.sh/v1alpha1") {
		t.Fatalf("manifest=%s", manifest)
	}
	if !strings.Contains(manifest, "minReplicaCount: 0") || !strings.Contains(summary, "Deployment/api") {
		t.Fatalf("summary=%s manifest=%s", summary, manifest)
	}
}

func TestGenerateScaledObjectRedis(t *testing.T) {
	manifest, _, err := GenerateScaledObject(ScaledObjectRequest{
		TargetName: "worker", Trigger: "redis", Queue: "jobs", Address: "redis:6379",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifest, "type: redis") || !strings.Contains(manifest, "listName: jobs") {
		t.Fatalf("manifest=%s", manifest)
	}
}

func TestDefaultScaledObjectName(t *testing.T) {
	if got := DefaultScaledObjectName("api", "cpu"); got != "api-keda" {
		t.Fatalf("got=%s", got)
	}
	if got := DefaultScaledObjectName("api", "redis"); got != "api-redis" {
		t.Fatalf("got=%s", got)
	}
}

func TestStatusFromObjectReady(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"metadata": map[string]any{"name": "api-keda", "namespace": "ns"},
		"status": map[string]any{
			"hpaName": "keda-hpa-api-keda",
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True", "message": "ScaledObject is defined correctly"},
			},
		},
	}}
	st := StatusFromObject(obj)
	if st.Phase != "Ready" || st.Name != "api-keda" || st.HPAName == "" {
		t.Fatalf("%+v", st)
	}
}
