package cluster

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

const discoveryCacheTTL = 5 * time.Minute

// DiscoveryClient is the subset of client-go discovery used for resource resolution.
type DiscoveryClient interface {
	ServerGroups() (*metav1.APIGroupList, error)
	ServerResourcesForGroupVersion(groupVersion string) (*metav1.APIResourceList, error)
}

// Resolver maps kind / plural / short name / group-qualified names to served API resources.
type Resolver struct {
	disc DiscoveryClient

	mu      sync.Mutex
	index   *resourceIndex
	loaded  time.Time
	partial []string // groups that failed discovery (informational)
}

// NewResolver builds a resolver over an injectable discovery client.
func NewResolver(disc DiscoveryClient) *Resolver {
	return &Resolver{disc: disc}
}

// NewResolverForConfig creates a discovery client from rest.Config.
func NewResolverForConfig(cfg *rest.Config) (*Resolver, error) {
	if cfg == nil {
		return nil, fmt.Errorf("rest config is nil")
	}
	dc, err := discovery.NewDiscoveryClientForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return NewResolver(dc), nil
}

// Invalidate drops the cached discovery index (call after CRD installs, etc.).
func (r *Resolver) Invalidate() {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.index = nil
	r.loaded = time.Time{}
	r.partial = nil
}

// PartialFailures returns API groups that failed during the last successful index build.
func (r *Resolver) PartialFailures() []string {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.partial...)
}

// Resolve looks up a user/LLM resource identity against cluster discovery.
func (r *Resolver) Resolve(ctx context.Context, query string) (ResourceRef, error) {
	_ = ctx
	query = strings.TrimSpace(query)
	if query == "" {
		return ResourceRef{}, fmt.Errorf("resource identity is required")
	}
	idx, err := r.loadIndex()
	if err != nil {
		return ResourceRef{}, err
	}
	return idx.resolve(query)
}

// ResolveRef fills Version / Scope / GVR fields on a partially parsed ResourceRef.
func (r *Resolver) ResolveRef(ctx context.Context, ref ResourceRef) (ResourceRef, error) {
	query := ref.Raw
	if query == "" {
		query = ref.Qualified()
	}
	if query == "" {
		query = ref.Kind
	}
	if query == "" {
		query = ref.Resource
	}
	resolved, err := r.Resolve(ctx, query)
	if err != nil {
		return ResourceRef{}, err
	}
	if ref.Raw != "" {
		resolved.Raw = ref.Raw
	}
	return resolved, nil
}

func (r *Resolver) loadIndex() (*resourceIndex, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.disc == nil {
		return nil, fmt.Errorf("discovery client is nil")
	}
	if r.index != nil && time.Since(r.loaded) < discoveryCacheTTL {
		return r.index, nil
	}
	idx, partial, err := buildResourceIndex(r.disc)
	if err != nil {
		return nil, err
	}
	r.index = idx
	r.loaded = time.Now()
	r.partial = partial
	return idx, nil
}

type resourceEntry struct {
	Ref            ResourceRef
	PreferredScore int // higher = better (preferred version)
}

type resourceIndex struct {
	// byKey maps lower-case lookup keys to candidate entries.
	byKey map[string][]resourceEntry
}

func buildResourceIndex(disc DiscoveryClient) (*resourceIndex, []string, error) {
	groups, err := disc.ServerGroups()
	if err != nil {
		return nil, nil, fmt.Errorf("discovery server groups: %w", err)
	}
	idx := &resourceIndex{byKey: map[string][]resourceEntry{}}
	var partial []string

	// Core/legacy group (v1) is not always listed the same way — also probe "" / "v1".
	coreListed := false
	for _, g := range groups.Groups {
		preferredGV := g.PreferredVersion.GroupVersion
		for _, v := range g.Versions {
			list, err := disc.ServerResourcesForGroupVersion(v.GroupVersion)
			if err != nil {
				if discovery.IsGroupDiscoveryFailedError(err) {
					partial = append(partial, v.GroupVersion)
					continue
				}
				// Keep other groups usable when one GV fails transiently.
				partial = append(partial, v.GroupVersion)
				continue
			}
			if g.Name == "" {
				coreListed = true
			}
			score := 0
			if v.GroupVersion == preferredGV {
				score = 10
			}
			idx.addAPIResourceList(g.Name, v.Version, list, score)
		}
	}
	if !coreListed {
		list, err := disc.ServerResourcesForGroupVersion("v1")
		if err == nil {
			idx.addAPIResourceList("", "v1", list, 10)
		} else if !discovery.IsGroupDiscoveryFailedError(err) {
			partial = append(partial, "v1")
		} else {
			partial = append(partial, "v1")
		}
	}

	if len(idx.byKey) == 0 {
		return nil, partial, fmt.Errorf("discovery returned no API resources")
	}
	return idx, partial, nil
}

func (idx *resourceIndex) addAPIResourceList(group, version string, list *metav1.APIResourceList, score int) {
	if list == nil {
		return
	}
	// Prefer group/version from the list header when present.
	gv := list.GroupVersion
	if gv != "" {
		if i := strings.Index(gv, "/"); i >= 0 {
			group = gv[:i]
			version = gv[i+1:]
		} else {
			group = ""
			version = gv
		}
	}
	for _, ar := range list.APIResources {
		if strings.Contains(ar.Name, "/") {
			continue // subresource
		}
		if !resourceReadable(ar) {
			continue
		}
		scope := ScopeCluster
		if ar.Namespaced {
			scope = ScopeNamespaced
		}
		ref := ResourceRef{
			Kind:     ar.Kind,
			Resource: ar.Name,
			Group:    group,
			Version:  version,
			Scope:    scope,
		}
		entry := resourceEntry{Ref: ref, PreferredScore: score}
		keys := []string{
			strings.ToLower(ar.Name),
			strings.ToLower(ar.Kind),
		}
		if group != "" {
			g := strings.ToLower(group)
			keys = append(keys,
				strings.ToLower(ar.Name)+"."+g,
				strings.ToLower(ar.Kind)+"."+g,
			)
		}
		for _, sn := range ar.ShortNames {
			keys = append(keys, strings.ToLower(sn))
			if group != "" {
				keys = append(keys, strings.ToLower(sn)+"."+strings.ToLower(group))
			}
		}
		for _, k := range keys {
			if k == "" || k == "." {
				continue
			}
			idx.byKey[k] = append(idx.byKey[k], entry)
		}
	}
}

func resourceReadable(ar metav1.APIResource) bool {
	if len(ar.Verbs) == 0 {
		return true
	}
	for _, v := range ar.Verbs {
		switch v {
		case "get", "list", "watch":
			return true
		}
	}
	return false
}

func (idx *resourceIndex) resolve(query string) (ResourceRef, error) {
	raw := query
	key := strings.ToLower(strings.TrimSpace(query))

	// Group-qualified: resource.group (may include dots in group, e.g. widgets.example.com).
	if i := strings.Index(key, "."); i > 0 {
		resource := key[:i]
		group := key[i+1:]
		exact := resource + "." + group
		if refs := idx.uniqueRefs(idx.byKey[exact]); len(refs) == 1 {
			out := refs[0]
			out.Raw = raw
			return out, nil
		}
		// Also try kind.group
		if refs := idx.uniqueRefs(idx.byKey[key]); len(refs) == 1 {
			out := refs[0]
			out.Raw = raw
			return out, nil
		}
		candidates := idx.uniqueRefs(idx.byKey[resource])
		filtered := filterByGroup(candidates, group)
		if len(filtered) == 1 {
			out := filtered[0]
			out.Raw = raw
			return out, nil
		}
		if len(filtered) > 1 {
			return ResourceRef{}, AmbiguousResourceError{Query: raw, Candidates: filtered}
		}
		return ResourceRef{}, UnknownResourceError{Ref: ResourceRef{Raw: raw, Resource: resource, Group: group}}
	}

	entries := idx.byKey[key]
	refs := idx.uniqueRefs(entries)
	switch len(refs) {
	case 0:
		return ResourceRef{}, UnknownResourceError{Ref: ResourceRef{Raw: raw}}
	case 1:
		out := refs[0]
		out.Raw = raw
		return out, nil
	default:
		return ResourceRef{}, AmbiguousResourceError{Query: raw, Candidates: refs}
	}
}

func (idx *resourceIndex) uniqueRefs(entries []resourceEntry) []ResourceRef {
	if len(entries) == 0 {
		return nil
	}
	type gr struct{ g, r string }
	best := map[gr]resourceEntry{}
	order := []gr{}
	for _, e := range entries {
		k := gr{e.Ref.Group, e.Ref.Resource}
		if prev, ok := best[k]; ok {
			if e.PreferredScore > prev.PreferredScore ||
				(e.PreferredScore == prev.PreferredScore && e.Ref.Version > prev.Ref.Version) {
				best[k] = e
			}
			continue
		}
		best[k] = e
		order = append(order, k)
	}
	out := make([]ResourceRef, 0, len(order))
	for _, k := range order {
		out = append(out, best[k].Ref)
	}
	return out
}

func filterByGroup(refs []ResourceRef, group string) []ResourceRef {
	group = strings.ToLower(group)
	var out []ResourceRef
	for _, r := range refs {
		if strings.ToLower(r.Group) == group {
			out = append(out, r)
		}
	}
	return out
}
