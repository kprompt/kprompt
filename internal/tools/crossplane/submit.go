package crossplane

import (
	"context"
	"fmt"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/yaml"
)

// Submit creates a Crossplane claim from manifest YAML and returns status.
func Submit(ctx context.Context, cfg *rest.Config, manifest string) (ClaimStatus, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return ClaimStatus{}, fmt.Errorf("crossplane dynamic client: %w", err)
	}
	return SubmitWithClient(ctx, dc, manifest)
}

// SubmitWithClient creates a claim using an injected dynamic client.
func SubmitWithClient(ctx context.Context, dc dynamic.Interface, manifest string) (ClaimStatus, error) {
	obj, gvr, ns, err := decodeClaimManifest(manifest)
	if err != nil {
		return ClaimStatus{}, err
	}
	created, err := dc.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{
		FieldManager: "kprompt",
	})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ClaimStatus{}, fmt.Errorf("claim %s/%s already exists", ns, obj.GetName())
		}
		return ClaimStatus{}, fmt.Errorf("create crossplane claim: %w", err)
	}
	return StatusFromObject(created), nil
}

func decodeClaimManifest(manifest string) (*unstructured.Unstructured, schema.GroupVersionResource, string, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("crossplane claim manifest is empty")
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &doc); err != nil {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("decode claim manifest: %w", err)
	}
	obj := &unstructured.Unstructured{Object: doc}
	if strings.TrimSpace(obj.GetKind()) == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("claim manifest missing kind")
	}
	gvr, err := gvrFromAPIVersion(obj.GetAPIVersion(), obj.GetKind())
	if err != nil {
		return nil, schema.GroupVersionResource{}, "", err
	}
	ns := strings.TrimSpace(obj.GetNamespace())
	if ns == "" {
		ns = "default"
		obj.SetNamespace(ns)
	}
	if strings.TrimSpace(obj.GetName()) == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("claim manifest missing metadata.name")
	}
	return obj, gvr, ns, nil
}

func gvrFromAPIVersion(apiVersion, kind string) (schema.GroupVersionResource, error) {
	gv, err := schema.ParseGroupVersion(strings.TrimSpace(apiVersion))
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("claim apiVersion: %w", err)
	}
	if gv.Group == "" || gv.Version == "" {
		return schema.GroupVersionResource{}, fmt.Errorf("claim apiVersion %q incomplete", apiVersion)
	}
	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: pluralizeKind(kind),
	}, nil
}

func pluralizeKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	if kind == "" {
		return "claims"
	}
	if strings.HasSuffix(kind, "s") {
		return kind
	}
	if strings.HasSuffix(kind, "y") && len(kind) > 1 {
		return kind[:len(kind)-1] + "ies"
	}
	return kind + "s"
}
