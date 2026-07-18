package cluster

import (
	"errors"
	"strings"
	"testing"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestParseResourceRefBuiltins(t *testing.T) {
	cases := []struct {
		raw, kind, resource, group string
		scope                      ResourceScope
	}{
		{"Pod", "Pod", "pods", "", ScopeNamespaced},
		{"pods", "Pod", "pods", "", ScopeNamespaced},
		{"po", "Pod", "pods", "", ScopeNamespaced},
		{"Deployment", "Deployment", "deployments", "apps", ScopeNamespaced},
		{"deployments.apps", "Deployment", "deployments", "apps", ScopeNamespaced},
		{"Node", "Node", "nodes", "", ScopeCluster},
		{"nodes", "Node", "nodes", "", ScopeCluster},
		{"ConfigMap", "ConfigMap", "configmaps", "", ScopeNamespaced},
		{"cm", "ConfigMap", "configmaps", "", ScopeNamespaced},
		{"Secret", "Secret", "secrets", "", ScopeNamespaced},
		{"secrets", "Secret", "secrets", "", ScopeNamespaced},
	}
	for _, tc := range cases {
		ref, err := ParseResourceRef(tc.raw)
		if err != nil {
			t.Fatalf("%q: %v", tc.raw, err)
		}
		if ref.Kind != tc.kind || ref.Resource != tc.resource || ref.Group != tc.group || ref.Scope != tc.scope {
			t.Fatalf("%q: got kind=%s resource=%s group=%s scope=%s",
				tc.raw, ref.Kind, ref.Resource, ref.Group, ref.Scope)
		}
	}
}

func TestParseResourceRefCRD(t *testing.T) {
	ref, err := ParseResourceRef("widgets.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Resource != "widgets" || ref.Group != "example.com" || ref.Kind != "Widget" {
		t.Fatalf("%+v", ref)
	}
	if ref.Scope != ScopeScopeUnknown {
		t.Fatalf("crd scope want unknown, got %s", ref.Scope)
	}
}

func TestNormalizeReadRequestClusterScoped(t *testing.T) {
	ref, err := ParseResourceRef("Node")
	if err != nil {
		t.Fatal(err)
	}
	req, err := NormalizeReadRequest(ReadRequest{
		Resource:  ref,
		Namespace: "default",
	})
	if err != nil {
		t.Fatal(err)
	}
	if req.Namespace != "" {
		t.Fatalf("cluster scope must clear namespace, got %q", req.Namespace)
	}
	if req.Limit != DefaultReadLimit {
		t.Fatalf("limit=%d", req.Limit)
	}
	if req.Timeout != DefaultReadTimeout {
		t.Fatalf("timeout=%s", req.Timeout)
	}

	_, err = NormalizeReadRequest(ReadRequest{
		Resource:  ref,
		Namespace: "kube-system",
	})
	if err == nil {
		t.Fatal("expected error for cluster-scoped + non-default namespace")
	}
}

func TestNormalizeReadRequestLimits(t *testing.T) {
	ref, _ := ParseResourceRef("Pod")
	_, err := NormalizeReadRequest(ReadRequest{Resource: ref, Namespace: "ns", Limit: MaxReadLimit + 1})
	if err == nil {
		t.Fatal("expected limit error")
	}
	_, err = NormalizeReadRequest(ReadRequest{
		Resource: ref,
		Namespace: "ns",
		Timeout:  3 * time.Minute,
	})
	if err == nil {
		t.Fatal("expected timeout error")
	}
}

func TestAmbiguousAndUnknownErrors(t *testing.T) {
	amb := AmbiguousResourceError{
		Query: "deploy",
		Candidates: []ResourceRef{
			{Resource: "deployments", Group: "apps", Kind: "Deployment"},
			{Resource: "deployments", Group: "extensions", Kind: "Deployment"},
		},
	}
	if !errors.Is(amb, ErrAmbiguousResource) {
		t.Fatal("unwrap ambiguous")
	}
	if !strings.Contains(amb.Error(), "deployments.apps") {
		t.Fatalf("msg=%s", amb.Error())
	}
	if msg := Friendlier(amb).Error(); !strings.Contains(msg, "ambiguous") {
		t.Fatalf("friendlier=%s", msg)
	}

	unk := UnknownResourceError{Ref: ResourceRef{Raw: "foobars"}}
	if !errors.Is(unk, ErrUnknownResource) {
		t.Fatal("unwrap unknown")
	}
	if msg := Friendlier(unk).Error(); !strings.Contains(msg, "unknown Kubernetes resource") {
		t.Fatalf("friendlier=%s", msg)
	}
}

func TestFriendlierRBAC(t *testing.T) {
	err := Friendlier(apierrors.NewForbidden(
		schema.GroupResource{Resource: "secrets"},
		"",
		errors.New(`User "alice" cannot list resource "secrets" in API group "" in the namespace "prod"`),
	))
	msg := err.Error()
	if !strings.Contains(msg, "RBAC denied") || !strings.Contains(msg, "secrets") {
		t.Fatalf("friendlier=%s", msg)
	}
	if !strings.Contains(msg, "kubectl auth can-i") {
		t.Fatalf("expected can-i hint, got %s", msg)
	}
}

func TestQueryFromReadRequest(t *testing.T) {
	ref, _ := ParseResourceRef("deployments.apps")
	req, err := NormalizeReadRequest(ReadRequest{
		Resource:      ref,
		Namespace:     "demo",
		LabelSelector: "app=api",
		Limit:         10,
		Continue:      "tok",
		Timeout:       15 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	q := QueryFromReadRequest(req)
	if q.Kind != "Deployment" || q.Group != "apps" || q.Resource != "deployments" {
		t.Fatalf("%+v", q)
	}
	if q.Namespace != "demo" || q.LabelSelector != "app=api" || q.Limit != 10 || q.Continue != "tok" {
		t.Fatalf("%+v", q)
	}
	if q.Timeout != 15*time.Second {
		t.Fatalf("timeout=%s", q.Timeout)
	}
}

func TestResultToReadTable(t *testing.T) {
	ref, _ := ParseResourceRef("Secret")
	req, _ := NormalizeReadRequest(ReadRequest{Resource: ref, Namespace: "ns"})
	table := Result{
		Kind:    "Secret",
		Headers: []string{"NAMESPACE", "NAME"},
		Rows:    []Row{{Namespace: "ns", Name: "db"}},
	}.ToReadTable(req)
	if table.Kind != "Secret" || table.Resource != "secrets" || table.Namespace != "ns" {
		t.Fatalf("%+v", table)
	}
	if len(table.Rows) != 1 || table.Rows[0]["NAME"] != "db" {
		t.Fatalf("rows=%v", table.Rows)
	}
}
