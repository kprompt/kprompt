package crossplane

import (
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGenerateClaimPostgres(t *testing.T) {
	manifest, summary, err := GenerateClaim(ClaimRequest{
		Name: "app-db", Namespace: "default", Resource: "postgres", StorageGB: 50, Provider: "aws",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifest, "kind: PostgreSQLInstance") || !strings.Contains(manifest, "storageGB: 50") {
		t.Fatalf("manifest=%s", manifest)
	}
	if !strings.Contains(summary, "strong approval") {
		t.Fatalf("summary=%s", summary)
	}
	if !strings.Contains(manifest, "provider: aws") {
		t.Fatalf("manifest=%s", manifest)
	}
}

func TestGenerateClaimBucket(t *testing.T) {
	manifest, _, err := GenerateClaim(ClaimRequest{Resource: "bucket", Name: "assets"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(manifest, "kind: Bucket") {
		t.Fatalf("manifest=%s", manifest)
	}
}

func TestStatusFromObjectReady(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"kind":     "PostgreSQLInstance",
		"metadata": map[string]any{"name": "db", "namespace": "ns"},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Synced", "status": "True"},
				map[string]any{"type": "Ready", "status": "True", "message": "Available"},
			},
		},
	}}
	st := StatusFromObject(obj)
	if st.Phase != "Ready" || st.Kind != "PostgreSQLInstance" {
		t.Fatalf("%+v", st)
	}
}

func TestPluralizeKind(t *testing.T) {
	if got := pluralizeKind("PostgreSQLInstance"); got != "postgresqlinstances" {
		t.Fatalf("got=%s", got)
	}
	if got := pluralizeKind("Bucket"); got != "buckets" {
		t.Fatalf("got=%s", got)
	}
}
