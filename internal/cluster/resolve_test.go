package cluster

import (
	"context"
	"errors"
	"fmt"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/discovery"
)

type staticDiscovery struct {
	groups    *metav1.APIGroupList
	resources map[string]*metav1.APIResourceList
	failGV    map[string]error
}

func (s *staticDiscovery) ServerGroups() (*metav1.APIGroupList, error) {
	if s.groups == nil {
		return &metav1.APIGroupList{}, nil
	}
	return s.groups, nil
}

func (s *staticDiscovery) ServerResourcesForGroupVersion(gv string) (*metav1.APIResourceList, error) {
	if s.failGV != nil {
		if err, ok := s.failGV[gv]; ok {
			return nil, err
		}
	}
	if s.resources == nil {
		return nil, fmt.Errorf("no resources for %s", gv)
	}
	list, ok := s.resources[gv]
	if !ok {
		return nil, fmt.Errorf("no resources for %s", gv)
	}
	return list, nil
}

func sampleDiscovery() *staticDiscovery {
	return &staticDiscovery{
		groups: &metav1.APIGroupList{
			Groups: []metav1.APIGroup{
				{
					Name: "",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "v1", Version: "v1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "v1", Version: "v1"},
				},
				{
					Name: "apps",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "apps/v1", Version: "v1"},
						{GroupVersion: "apps/v1beta1", Version: "v1beta1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "apps/v1", Version: "v1"},
				},
				{
					Name: "example.com",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "example.com/v1", Version: "v1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "example.com/v1", Version: "v1"},
				},
				{
					Name: "conflict.example.com",
					Versions: []metav1.GroupVersionForDiscovery{
						{GroupVersion: "conflict.example.com/v1", Version: "v1"},
					},
					PreferredVersion: metav1.GroupVersionForDiscovery{GroupVersion: "conflict.example.com/v1", Version: "v1"},
				},
			},
		},
		resources: map[string]*metav1.APIResourceList{
			"v1": {
				GroupVersion: "v1",
				APIResources: []metav1.APIResource{
					{Name: "pods", SingularName: "pod", Namespaced: true, Kind: "Pod", ShortNames: []string{"po"}, Verbs: []string{"get", "list"}},
					{Name: "nodes", SingularName: "node", Namespaced: false, Kind: "Node", ShortNames: []string{"no"}, Verbs: []string{"get", "list"}},
					{Name: "configmaps", SingularName: "configmap", Namespaced: true, Kind: "ConfigMap", ShortNames: []string{"cm"}, Verbs: []string{"get", "list"}},
					{Name: "secrets", SingularName: "secret", Namespaced: true, Kind: "Secret", Verbs: []string{"get", "list"}},
					{Name: "pods/log", Namespaced: true, Kind: "Pod", Verbs: []string{"get"}},
				},
			},
			"apps/v1": {
				GroupVersion: "apps/v1",
				APIResources: []metav1.APIResource{
					{Name: "deployments", SingularName: "deployment", Namespaced: true, Kind: "Deployment", ShortNames: []string{"deploy"}, Verbs: []string{"get", "list"}},
				},
			},
			"apps/v1beta1": {
				GroupVersion: "apps/v1beta1",
				APIResources: []metav1.APIResource{
					{Name: "deployments", SingularName: "deployment", Namespaced: true, Kind: "Deployment", ShortNames: []string{"deploy"}, Verbs: []string{"get", "list"}},
				},
			},
			"example.com/v1": {
				GroupVersion: "example.com/v1",
				APIResources: []metav1.APIResource{
					{Name: "widgets", SingularName: "widget", Namespaced: true, Kind: "Widget", Verbs: []string{"get", "list"}},
				},
			},
			"conflict.example.com/v1": {
				GroupVersion: "conflict.example.com/v1",
				APIResources: []metav1.APIResource{
					{Name: "deployables", SingularName: "deployable", Namespaced: true, Kind: "Deployable", ShortNames: []string{"deploy"}, Verbs: []string{"get", "list"}},
				},
			},
		},
	}
}

func TestResolverBuiltinsAndAliases(t *testing.T) {
	r := NewResolver(sampleDiscovery())
	cases := []struct {
		query, kind, resource, group, version string
		scope                                 ResourceScope
	}{
		{"node", "Node", "nodes", "", "v1", ScopeCluster},
		{"nodes", "Node", "nodes", "", "v1", ScopeCluster},
		{"no", "Node", "nodes", "", "v1", ScopeCluster},
		{"Pod", "Pod", "pods", "", "v1", ScopeNamespaced},
		{"po", "Pod", "pods", "", "v1", ScopeNamespaced},
		{"configmaps", "ConfigMap", "configmaps", "", "v1", ScopeNamespaced},
		{"cm", "ConfigMap", "configmaps", "", "v1", ScopeNamespaced},
		{"secrets", "Secret", "secrets", "", "v1", ScopeNamespaced},
		{"deployments.apps", "Deployment", "deployments", "apps", "v1", ScopeNamespaced},
		{"Deployment.apps", "Deployment", "deployments", "apps", "v1", ScopeNamespaced},
	}
	for _, tc := range cases {
		ref, err := r.Resolve(context.Background(), tc.query)
		if err != nil {
			t.Fatalf("%q: %v", tc.query, err)
		}
		if ref.Kind != tc.kind || ref.Resource != tc.resource || ref.Group != tc.group || ref.Version != tc.version || ref.Scope != tc.scope {
			t.Fatalf("%q: got %+v want kind=%s resource=%s group=%s version=%s scope=%s",
				tc.query, ref, tc.kind, tc.resource, tc.group, tc.version, tc.scope)
		}
	}
}

func TestResolverPreferredVersion(t *testing.T) {
	r := NewResolver(sampleDiscovery())
	ref, err := r.Resolve(context.Background(), "deployments")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Version != "v1" {
		t.Fatalf("preferred version want v1, got %s", ref.Version)
	}
}

func TestResolverCRD(t *testing.T) {
	r := NewResolver(sampleDiscovery())
	ref, err := r.Resolve(context.Background(), "widgets.example.com")
	if err != nil {
		t.Fatal(err)
	}
	if ref.Kind != "Widget" || ref.Group != "example.com" || ref.Version != "v1" {
		t.Fatalf("%+v", ref)
	}
}

func TestResolverAmbiguousShortName(t *testing.T) {
	r := NewResolver(sampleDiscovery())
	_, err := r.Resolve(context.Background(), "deploy")
	if err == nil {
		t.Fatal("expected ambiguous")
	}
	var amb AmbiguousResourceError
	if !errors.As(err, &amb) {
		t.Fatalf("got %T %v", err, err)
	}
	if len(amb.Candidates) < 2 {
		t.Fatalf("candidates=%v", amb.Candidates)
	}
}

func TestResolverUnknown(t *testing.T) {
	r := NewResolver(sampleDiscovery())
	_, err := r.Resolve(context.Background(), "foobars")
	if !errors.Is(err, ErrUnknownResource) {
		t.Fatalf("got %v", err)
	}
}

func TestResolverPartialDiscoveryFailure(t *testing.T) {
	disc := sampleDiscovery()
	disc.failGV = map[string]error{
		"conflict.example.com/v1": &discovery.ErrGroupDiscoveryFailed{
			Groups: map[schema.GroupVersion]error{
				{Group: "conflict.example.com", Version: "v1"}: fmt.Errorf("unavailable"),
			},
		},
	}

	r := NewResolver(disc)
	ref, err := r.Resolve(context.Background(), "pods")
	if err != nil {
		t.Fatalf("healthy group should resolve: %v", err)
	}
	if ref.Kind != "Pod" {
		t.Fatalf("%+v", ref)
	}
	if len(r.PartialFailures()) == 0 {
		t.Fatal("expected partial failure recorded")
	}
	ref, err = r.Resolve(context.Background(), "deploy")
	if err != nil {
		t.Fatalf("deploy should be unique after partial failure: %v", err)
	}
	if ref.Resource != "deployments" {
		t.Fatalf("%+v", ref)
	}
}

func TestResolverInvalidate(t *testing.T) {
	r := NewResolver(sampleDiscovery())
	if _, err := r.Resolve(context.Background(), "pods"); err != nil {
		t.Fatal(err)
	}
	r.Invalidate()
	if r.index != nil {
		t.Fatal("index should be cleared")
	}
}

func TestResolverResolveRef(t *testing.T) {
	r := NewResolver(sampleDiscovery())
	parsed, err := ParseResourceRef("nodes")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := r.ResolveRef(context.Background(), parsed)
	if err != nil {
		t.Fatal(err)
	}
	if ref.Version != "v1" || ref.Scope != ScopeCluster {
		t.Fatalf("%+v", ref)
	}
}
