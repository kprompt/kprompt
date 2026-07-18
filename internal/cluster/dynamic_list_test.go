package cluster

import (
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestReaderDynamicNodes(t *testing.T) {
	node := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Node",
		"metadata": map[string]any{
			"name":              "worker-1",
			"creationTimestamp": metav1.NewTime(time.Now().Add(-2 * time.Hour)).Format(time.RFC3339),
		},
		"status": map[string]any{
			"conditions": []any{
				map[string]any{"type": "Ready", "status": "True"},
			},
		},
	}}
	dc := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "nodes"}: "NodeList",
		},
		node,
	)
	r := &Reader{Dynamic: dc}
	res, err := r.List(context.Background(), Query{
		Kind: "Node", Group: "", Version: "v1", Resource: "nodes", Scope: ScopeCluster,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Name != "worker-1" {
		t.Fatalf("%+v", res.Rows)
	}
	if res.Rows[0].Status != "Ready" {
		t.Fatalf("status=%s", res.Rows[0].Status)
	}
	if strings.Contains(strings.Join(res.Headers, ","), "NAMESPACE") {
		t.Fatalf("cluster scope should omit namespace header: %v", res.Headers)
	}
}

func TestReaderDynamicConfigMapAndSecret(t *testing.T) {
	cm := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata":   map[string]any{"name": "app", "namespace": "demo"},
		"data":       map[string]any{"a": "1", "b": "2"},
	}}
	sec := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata":   map[string]any{"name": "db", "namespace": "demo"},
		"type":       "Opaque",
		"data":       map[string]any{"password": "c2VjcmV0"}, // base64 "secret" — table must not require decode
	}}
	dc := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "configmaps"}: "ConfigMapList",
			{Group: "", Version: "v1", Resource: "secrets"}:    "SecretList",
		},
		cm, sec,
	)
	r := &Reader{Dynamic: dc}

	cmRes, err := r.List(context.Background(), Query{
		Kind: "ConfigMap", Namespace: "demo", Version: "v1", Resource: "configmaps", Scope: ScopeNamespaced,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cmRes.Rows) != 1 || cmRes.Rows[0].Status != "2" {
		t.Fatalf("configmap rows=%+v", cmRes.Rows)
	}

	secRes, err := r.List(context.Background(), Query{
		Kind: "Secret", Namespace: "demo", Name: "db", Version: "v1", Resource: "secrets", Scope: ScopeNamespaced,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(secRes.Rows) != 1 || !strings.Contains(secRes.Rows[0].Status, "Opaque") {
		t.Fatalf("secret rows=%+v", secRes.Rows)
	}
	// No secret payload in table cells.
	joined := secRes.Rows[0].Name + secRes.Rows[0].Status + secRes.Rows[0].Extra
	if strings.Contains(joined, "c2VjcmV0") {
		t.Fatal("secret data value leaked into table")
	}
}

func TestReaderDynamicCRD(t *testing.T) {
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"metadata":   map[string]any{"name": "w1", "namespace": "demo"},
		"status":     map[string]any{"phase": "Ready"},
	}}
	dc := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "example.com", Version: "v1", Resource: "widgets"}: "WidgetList",
		},
		obj,
	)
	r := &Reader{Dynamic: dc}
	res, err := r.List(context.Background(), Query{
		Kind: "Widget", Namespace: "demo", Group: "example.com", Version: "v1", Resource: "widgets", Scope: ScopeNamespaced,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Status != "Ready" {
		t.Fatalf("%+v", res)
	}
}

func TestReaderDynamicRespectsLimit(t *testing.T) {
	objs := []runtime.Object{
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "a"}, "status": map[string]any{"phase": "Active"}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "b"}, "status": map[string]any{"phase": "Active"}}},
		&unstructured.Unstructured{Object: map[string]any{"apiVersion": "v1", "kind": "Namespace", "metadata": map[string]any{"name": "c"}, "status": map[string]any{"phase": "Active"}}},
	}
	dc := fake.NewSimpleDynamicClientWithCustomListKinds(
		runtime.NewScheme(),
		map[schema.GroupVersionResource]string{
			{Group: "", Version: "v1", Resource: "namespaces"}: "NamespaceList",
		},
		objs...,
	)
	r := &Reader{Dynamic: dc}
	res, err := r.List(context.Background(), Query{
		Kind: "Namespace", Version: "v1", Resource: "namespaces", Scope: ScopeCluster, Limit: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 2 || !res.Truncated {
		t.Fatalf("rows=%d truncated=%v", len(res.Rows), res.Truncated)
	}
}

func TestReaderTypedPodsStillWork(t *testing.T) {
	client := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "p1", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})
	r := &Reader{Client: client}
	res, err := r.List(context.Background(), Query{Kind: "Pod", Namespace: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Rows) != 1 || res.Rows[0].Name != "p1" {
		t.Fatalf("%+v", res.Rows)
	}
}

func TestReaderPrefersTypedOverDynamicForPods(t *testing.T) {
	client := k8sfake.NewSimpleClientset(&corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "typed", Namespace: "default"},
		Status:     corev1.PodStatus{Phase: corev1.PodRunning},
	})
	dc := fake.NewSimpleDynamicClient(runtime.NewScheme())
	r := &Reader{Client: client, Dynamic: dc}
	res, err := r.List(context.Background(), Query{Kind: "pods", Namespace: "default"})
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Headers, "RESTARTS") {
		t.Fatalf("expected typed headers, got %v", res.Headers)
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
