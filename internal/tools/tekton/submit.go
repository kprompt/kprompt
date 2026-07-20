package tekton

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

// PipelineRunGVR is the namespaced resource for Tekton PipelineRuns.
var PipelineRunGVR = schema.GroupVersionResource{
	Group:    PipelineGroup,
	Version:  "v1",
	Resource: "pipelineruns",
}

// Submit creates a PipelineRun from manifest YAML and returns its current status.
func Submit(ctx context.Context, cfg *rest.Config, manifest string) (PipelineRunStatus, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return PipelineRunStatus{}, fmt.Errorf("tekton dynamic client: %w", err)
	}
	return SubmitWithClient(ctx, dc, manifest)
}

// SubmitWithClient creates a PipelineRun using an injected dynamic client.
func SubmitWithClient(ctx context.Context, dc dynamic.Interface, manifest string) (PipelineRunStatus, error) {
	obj, gvr, ns, err := decodePipelineRunManifest(manifest)
	if err != nil {
		return PipelineRunStatus{}, err
	}
	created, err := dc.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{
		FieldManager: "kprompt",
	})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return PipelineRunStatus{}, fmt.Errorf("pipelinerun %s/%s already exists", ns, obj.GetName())
		}
		return PipelineRunStatus{}, fmt.Errorf("create pipelinerun: %w", err)
	}
	return StatusFromObject(created), nil
}

// GetStatus reads the current phase for a PipelineRun by name.
func GetStatus(ctx context.Context, cfg *rest.Config, namespace, name string) (PipelineRunStatus, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return PipelineRunStatus{}, fmt.Errorf("tekton dynamic client: %w", err)
	}
	return GetStatusWithClient(ctx, dc, namespace, name)
}

// GetStatusWithClient reads PipelineRun status using an injected dynamic client.
func GetStatusWithClient(ctx context.Context, dc dynamic.Interface, namespace, name string) (PipelineRunStatus, error) {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return PipelineRunStatus{}, fmt.Errorf("pipelinerun name is required")
	}
	obj, err := dc.Resource(PipelineRunGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return PipelineRunStatus{}, fmt.Errorf("get pipelinerun: %w", err)
	}
	return StatusFromObject(obj), nil
}

func decodePipelineRunManifest(manifest string) (*unstructured.Unstructured, schema.GroupVersionResource, string, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("pipelinerun manifest is empty")
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &doc); err != nil {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("decode pipelinerun manifest: %w", err)
	}
	obj := &unstructured.Unstructured{Object: doc}
	kind := strings.TrimSpace(obj.GetKind())
	if kind == "" {
		kind = PipelineRunKind
		obj.SetKind(kind)
	}
	if !strings.EqualFold(kind, PipelineRunKind) {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("manifest kind %q is not PipelineRun", kind)
	}
	gvr, err := pipelineRunGVRFromAPIVersion(obj.GetAPIVersion())
	if err != nil {
		return nil, schema.GroupVersionResource{}, "", err
	}
	ns := strings.TrimSpace(obj.GetNamespace())
	if ns == "" {
		ns = "default"
		obj.SetNamespace(ns)
	}
	if strings.TrimSpace(obj.GetName()) == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("pipelinerun manifest missing metadata.name")
	}
	return obj, gvr, ns, nil
}

func pipelineRunGVRFromAPIVersion(apiVersion string) (schema.GroupVersionResource, error) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return PipelineRunGVR, nil
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("pipelinerun apiVersion: %w", err)
	}
	if gv.Group != PipelineGroup {
		return schema.GroupVersionResource{}, fmt.Errorf("pipelinerun apiVersion group %q unexpected", gv.Group)
	}
	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: "pipelineruns",
	}, nil
}
