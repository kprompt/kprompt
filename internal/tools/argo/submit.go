package argo

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

// WorkflowGVR is the cluster-scoped resource for Argo Workflows.
var WorkflowGVR = schema.GroupVersionResource{
	Group:    WorkflowGroup,
	Version:  "v1alpha1",
	Resource: "workflows",
}

// Submit creates a Workflow from manifest YAML and returns its current status.
func Submit(ctx context.Context, cfg *rest.Config, manifest string) (WorkflowStatus, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return WorkflowStatus{}, fmt.Errorf("argo dynamic client: %w", err)
	}
	return SubmitWithClient(ctx, dc, manifest)
}

// SubmitWithClient creates a Workflow using an injected dynamic client.
func SubmitWithClient(ctx context.Context, dc dynamic.Interface, manifest string) (WorkflowStatus, error) {
	obj, gvr, ns, err := decodeWorkflowManifest(manifest)
	if err != nil {
		return WorkflowStatus{}, err
	}
	created, err := dc.Resource(gvr).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{
		FieldManager: "kprompt",
	})
	if err != nil {
		if apierrors.IsAlreadyExists(err) {
			return WorkflowStatus{}, fmt.Errorf("workflow %s/%s already exists", ns, obj.GetName())
		}
		return WorkflowStatus{}, fmt.Errorf("create workflow: %w", err)
	}
	return StatusFromObject(created), nil
}

// GetStatus reads the current phase for a Workflow by name.
func GetStatus(ctx context.Context, cfg *rest.Config, namespace, name string) (WorkflowStatus, error) {
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return WorkflowStatus{}, fmt.Errorf("argo dynamic client: %w", err)
	}
	return GetStatusWithClient(ctx, dc, namespace, name)
}

// GetStatusWithClient reads workflow status using an injected dynamic client.
func GetStatusWithClient(ctx context.Context, dc dynamic.Interface, namespace, name string) (WorkflowStatus, error) {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return WorkflowStatus{}, fmt.Errorf("workflow name is required")
	}
	obj, err := dc.Resource(WorkflowGVR).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return WorkflowStatus{}, fmt.Errorf("get workflow: %w", err)
	}
	return StatusFromObject(obj), nil
}

func decodeWorkflowManifest(manifest string) (*unstructured.Unstructured, schema.GroupVersionResource, string, error) {
	manifest = strings.TrimSpace(manifest)
	if manifest == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("workflow manifest is empty")
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(manifest), &doc); err != nil {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("decode workflow manifest: %w", err)
	}
	obj := &unstructured.Unstructured{Object: doc}
	kind := strings.TrimSpace(obj.GetKind())
	if kind == "" {
		kind = WorkflowKind
		obj.SetKind(kind)
	}
	if !strings.EqualFold(kind, WorkflowKind) {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("manifest kind %q is not Workflow", kind)
	}
	gvr, err := workflowGVRFromAPIVersion(obj.GetAPIVersion())
	if err != nil {
		return nil, schema.GroupVersionResource{}, "", err
	}
	ns := strings.TrimSpace(obj.GetNamespace())
	if ns == "" {
		ns = "default"
		obj.SetNamespace(ns)
	}
	if strings.TrimSpace(obj.GetName()) == "" {
		return nil, schema.GroupVersionResource{}, "", fmt.Errorf("workflow manifest missing metadata.name")
	}
	return obj, gvr, ns, nil
}

func workflowGVRFromAPIVersion(apiVersion string) (schema.GroupVersionResource, error) {
	apiVersion = strings.TrimSpace(apiVersion)
	if apiVersion == "" {
		return WorkflowGVR, nil
	}
	gv, err := schema.ParseGroupVersion(apiVersion)
	if err != nil {
		return schema.GroupVersionResource{}, fmt.Errorf("workflow apiVersion: %w", err)
	}
	if gv.Group != WorkflowGroup {
		return schema.GroupVersionResource{}, fmt.Errorf("workflow apiVersion group %q unexpected", gv.Group)
	}
	return schema.GroupVersionResource{
		Group:    gv.Group,
		Version:  gv.Version,
		Resource: "workflows",
	}, nil
}
