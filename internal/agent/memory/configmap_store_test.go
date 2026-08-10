package memory

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestConfigMapStoreSaveLoad(t *testing.T) {
	client := fake.NewSimpleClientset()
	store := ConfigMapStore{Client: client, Namespace: "payments"}

	// Load before the ConfigMap exists surfaces the NotFound error.
	if _, err := store.Load("payments"); err == nil {
		t.Fatal("expected NotFound before any save")
	}

	// Save creates the ConfigMap.
	in := Snapshot{Namespace: "payments", Facts: []Fact{{ID: "dependency/redis", Kind: KindDependency, Key: "redis"}}}
	if err := store.Save(in); err != nil {
		t.Fatalf("save create: %v", err)
	}
	got, err := store.Load("payments")
	if err != nil {
		t.Fatalf("load after create: %v", err)
	}
	if len(got.Facts) != 1 || got.Facts[0].Key != "redis" {
		t.Fatalf("roundtrip mismatch: %+v", got.Facts)
	}

	// Save again updates the existing ConfigMap (create branch already covered).
	in.Facts = append(in.Facts, Fact{ID: "dependency/kafka", Kind: KindDependency, Key: "kafka"})
	if err := store.Save(in); err != nil {
		t.Fatalf("save update: %v", err)
	}
	got, _ = store.Load("payments")
	if len(got.Facts) != 2 {
		t.Fatalf("expected 2 facts after update, got %d", len(got.Facts))
	}
}

func TestConfigMapStoreNamespaceOverride(t *testing.T) {
	client := fake.NewSimpleClientset()
	// Store Namespace pins writes regardless of the argument namespace.
	store := ConfigMapStore{Client: client, Namespace: "pinned"}
	if err := store.Save(Snapshot{Namespace: "ignored", Facts: []Fact{{Key: "redis", Kind: KindDependency}}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	if _, err := client.CoreV1().ConfigMaps("pinned").Get(context.Background(), ConfigMapName, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected ConfigMap in pinned ns: %v", err)
	}
}

func TestConfigMapStoreListNamespaces(t *testing.T) {
	mk := func(ns string) *corev1.ConfigMap {
		return &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ConfigMapName,
				Namespace: ns,
				Labels: map[string]string{
					"app.kubernetes.io/component":  "namespace-memory",
					"app.kubernetes.io/managed-by": "kprompt",
				},
			},
			Data: map[string]string{ConfigMapKey: "{}"},
		}
	}
	client := fake.NewSimpleClientset(mk("a"), mk("b"))
	store := ConfigMapStore{Client: client}
	nsList, err := store.ListNamespaces()
	if err != nil {
		t.Fatalf("list namespaces: %v", err)
	}
	if len(nsList) != 2 {
		t.Fatalf("expected 2 namespaces, got %v", nsList)
	}
}
