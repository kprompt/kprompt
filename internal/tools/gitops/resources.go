package gitops

import (
	"context"
	"fmt"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// ResourceDrift is one drifted (or parent-out-of-sync inventory) child resource.
// Argo: status.resources entries that are not Synced.
// Flux: status.inventory.entries while the Kustomization is OutOfSync (Flux has no
// per-resource sync flag — inventory is the managed set; inherit OutOfSync).
type ResourceDrift struct {
	APIVersion string `json:"apiVersion,omitempty"`
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	Namespace  string `json:"namespace,omitempty"`
	Status     string `json:"status,omitempty"`
	Health     string `json:"health,omitempty"`
}

// ListResourceDrifts returns OutOfSync (or otherwise non-Synced) child resources
// for Argo CD Applications, or inventory entries for out-of-sync Flux Kustomizations.
func ListResourceDrifts(ctx context.Context, cfg *rest.Config, app AppStatus) ([]ResourceDrift, error) {
	engine := strings.ToLower(strings.TrimSpace(app.Engine))
	switch engine {
	case "argocd", "argo":
		return listArgoResourceDrifts(ctx, cfg, app)
	case "flux":
		return listFluxResourceDrifts(ctx, cfg, app)
	default:
		return nil, nil
	}
}

func listArgoResourceDrifts(ctx context.Context, cfg *rest.Config, app AppStatus) ([]ResourceDrift, error) {
	if cfg == nil {
		return nil, fmt.Errorf("gitops resource drifts: rest config is nil")
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	obj, err := dc.Resource(ApplicationGVR).Namespace(app.Namespace).Get(ctx, app.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return resourceDriftsFromArgoApp(obj), nil
}

func listFluxResourceDrifts(ctx context.Context, cfg *rest.Config, app AppStatus) ([]ResourceDrift, error) {
	// HelmRelease has no Kustomization-style inventory.entries — degrade honestly.
	if kind := strings.TrimSpace(app.Kind); kind != "" && !strings.EqualFold(kind, FluxKind) && !strings.EqualFold(kind, "Kustomization") {
		return nil, fmt.Errorf("flux inventory unavailable for kind %s (Kustomization status.inventory only)", kind)
	}
	if cfg == nil {
		return nil, fmt.Errorf("gitops resource drifts: rest config is nil")
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	obj, err := dc.Resource(KustomizationGVR).Namespace(app.Namespace).Get(ctx, app.Name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return resourceDriftsFromFluxKustomization(obj)
}

func resourceDriftsFromArgoApp(obj *unstructured.Unstructured) []ResourceDrift {
	if obj == nil {
		return nil
	}
	raw, ok, _ := unstructured.NestedSlice(obj.Object, "status", "resources")
	if !ok {
		return nil
	}
	out := make([]ResourceDrift, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		status, _ := m["status"].(string)
		status = strings.TrimSpace(status)
		if status == "" || strings.EqualFold(status, "Synced") {
			continue
		}
		kind, _ := m["kind"].(string)
		name, _ := m["name"].(string)
		if strings.TrimSpace(kind) == "" || strings.TrimSpace(name) == "" {
			continue
		}
		ns, _ := m["namespace"].(string)
		api, _ := m["version"].(string)
		if g, _ := m["group"].(string); g != "" {
			if api != "" {
				api = g + "/" + api
			} else {
				api = g
			}
		}
		health := ""
		if hm, ok, _ := unstructured.NestedString(m, "health", "status"); ok {
			health = hm
		}
		out = append(out, ResourceDrift{
			APIVersion: api,
			Kind:       kind,
			Name:       name,
			Namespace:  ns,
			Status:     status,
			Health:     health,
		})
	}
	return out
}

// resourceDriftsFromFluxKustomization expands status.inventory.entries when present.
// Returns an error when the inventory field is absent so callers can degrade honestly.
func resourceDriftsFromFluxKustomization(obj *unstructured.Unstructured) ([]ResourceDrift, error) {
	if obj == nil {
		return nil, fmt.Errorf("flux inventory unavailable")
	}
	inv, found, err := unstructured.NestedMap(obj.Object, "status", "inventory")
	if err != nil || !found || inv == nil {
		return nil, fmt.Errorf("flux inventory unavailable")
	}
	raw, ok, _ := unstructured.NestedSlice(inv, "entries")
	if !ok {
		// inventory present but empty/malformed entries — treat as empty list, not degrade
		return nil, nil
	}
	out := make([]ResourceDrift, 0, len(raw))
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		ver, _ := m["v"].(string)
		ns, name, group, kind, ok := parseFluxInventoryID(id)
		if !ok {
			continue
		}
		api := strings.TrimSpace(ver)
		if group != "" {
			if api != "" {
				api = group + "/" + api
			} else {
				api = group
			}
		}
		out = append(out, ResourceDrift{
			APIVersion: api,
			Kind:       kind,
			Name:       name,
			Namespace:  ns,
			// Flux has no per-resource sync bit; parent OutOfSync → inherit.
			Status: "OutOfSync",
		})
	}
	return out, nil
}

// parseFluxInventoryID parses SSA ObjMetadata ids: namespace_name_group_kind
// (empty group yields a double underscore before kind).
func parseFluxInventoryID(id string) (ns, name, group, kind string, ok bool) {
	id = strings.TrimSpace(id)
	if id == "" {
		return "", "", "", "", false
	}
	// kind
	i := strings.LastIndex(id, "_")
	if i < 0 {
		return "", "", "", "", false
	}
	kind = id[i+1:]
	rest := id[:i]
	// group (may be empty)
	i = strings.LastIndex(rest, "_")
	if i < 0 {
		return "", "", "", "", false
	}
	group = rest[i+1:]
	rest = rest[:i]
	// name
	i = strings.LastIndex(rest, "_")
	if i < 0 {
		// cluster-scoped with empty ns encoded as leading underscore already consumed
		// rest is name only when format was _name__Kind → after stripping kind+group, rest="_name" or "name"
		name = strings.TrimPrefix(rest, "_")
		if name == "" || kind == "" {
			return "", "", "", "", false
		}
		return "", name, group, kind, true
	}
	name = rest[i+1:]
	ns = rest[:i]
	if name == "" || kind == "" {
		return "", "", "", "", false
	}
	return ns, name, group, kind, true
}
