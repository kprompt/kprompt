package keda

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

// ScaledObjectGVR is the namespaced resource for KEDA ScaledObjects.
var ScaledObjectGVR = schema.GroupVersionResource{
	Group:    ScaledObjectGroup,
	Version:  "v1alpha1",
	Resource: "scaledobjects",
}

// Submit creates a ScaledObject from manifest YAML and returns its current status.
func Submit(ctx context.Context, cfg *rest.Config, manifest string) (ScaledObjectStatus, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return ScaledObjectStatus{}, fmt.Errorf("keda dynamic client: %w", err)
	}
	return SubmitWithClient(ctx, dc, manifest)
}

// SubmitWithClient creates a ScaledObject using an injected dynamic client.
func SubmitWithClient(ctx context.Context, dc dynamic.Interface, manifest string) (ScaledObjectStatus, error) {
	obj, gvr, ns, err := decodeScaledObjectManifest(manifest)
	if err != nil {
		return ScaledObjectStatus{}, err
	}
	created, err := dc.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{
		FieldManager: "kprompt",
	})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return ScaledObjectStatus{}, fmt.Errorf("scaledobject %s/%s already exists", ns, obj.GetName())
		}
		return ScaledObjectStatus{}, fmt.Errorf("create scaledobject: %w", err)
	}
	return StatusFromObject(created), nil
}

func decodeScaledObjectManifest(manifest string) (*unstructured.Unstructured, schema.GroupVersionResource, string, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("scaledobject manifest is empty")
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &doc); err != nil {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("decode scaledobject manifest: %w", err)
	}
	obj := &unstructured.Unstructured{Object: doc}
	kind := strings.TrimSpace(obj.GetKind())
	if kind == "" {
		kind = ScaledObjectKind
		obj.SetKind(kind)
	}
	if !strings.EqualFold(kind, ScaledObjectKind) {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("manifest kind %q is not ScaledObject", kind)
	}
	gvr, err := scaledObjectGVRFromAPIVersion(obj.GetAPIVersion())
	if err != nil {
		return nil, schema.GroupVersionResource{}, "", err
	}
	ns := strings.TrimSpace(obj.GetNamespace())
	if ns == "" {
		ns = "default"
		obj.SetNamespace(ns)
	}
	if strings.TrimSpace(obj.GetName()) == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("scaledobject manifest missing metadata.name")
	}
	return obj, gvr, ns, nil
}

func scaledObjectGVRFromAPIVersion(apiVersion string) (schema.GroupVersionResource, error) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return ScaledObjectGVR, nil
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("scaledobject apiVersion: %w", err)
	}
	if gv.Group != ScaledObjectGroup {
		return schema.GroupVersionResource{}, fmt.Errorf("scaledobject apiVersion group %q unexpected", gv.Group)
	}
	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: "scaledobjects",
	}, nil
}
