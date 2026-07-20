package gitops

import (
	"context"
	"fmt"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// SyncRequest triggers a GitOps reconcile/sync on a named app.
type SyncRequest struct {
	Engine    string // flux | argocd
	Name      string
	Namespace string
	Action    string // sync | promote | rollback
	Revision  string // optional target revision for Argo CD
}

// SyncResult is the outcome of a sync/reconcile trigger.
type SyncResult struct {
	Engine    string `json:"engine"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Action    string `json:"action"`
	Message   string `json:"message"`
}

// TriggerSync annotates Flux Kustomization or patches Argo CD Application for reconcile.
func TriggerSync(ctx context.Context, cfg *rest.Config, req SyncRequest) (SyncResult, error) {
	if cfg == nil {
		return SyncResult{}, fmt.Errorf("gitops sync: rest config is nil")
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return SyncResult{}, fmt.Errorf("gitops dynamic client: %w", err)
	}
	return TriggerSyncWithClient(ctx, dc, req)
}

// TriggerSyncWithClient applies a sync/reconcile using an injected dynamic client.
func TriggerSyncWithClient(ctx context.Context, dc dynamic.Interface, req SyncRequest) (SyncResult, error) {
	req.Name = strings.TrimSpace(req.Name)
	req.Namespace = strings.TrimSpace(req.Namespace)
	req.Engine = strings.ToLower(strings.TrimSpace(req.Engine))
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	if req.Action == "" {
		req.Action = "sync"
	}
	if req.Name == "" {
		return SyncResult{}, fmt.Errorf("gitops sync requires a named Application or Kustomization")
	}
	if req.Namespace == "" {
		req.Namespace = "default"
	}
	if req.Engine == "" || req.Engine == "auto" {
		return SyncResult{}, fmt.Errorf("gitops sync requires engine=flux or engine=argocd")
	}

	switch req.Engine {
	case "flux":
		return syncFlux(ctx, dc, req)
	case "argocd", "argo":
		return syncArgoCD(ctx, dc, req)
	default:
		return SyncResult{}, fmt.Errorf("unsupported gitops engine %q", req.Engine)
	}
}

func syncFlux(ctx context.Context, dc dynamic.Interface, req SyncRequest) (SyncResult, error) {
	obj, err := dc.Resource(KustomizationGVR).Namespace(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return SyncResult{}, fmt.Errorf("get kustomization: %w", err)
	}
	anns := obj.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns["reconcile.fluxcd.io/requestedAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	obj.SetAnnotations(anns)
	updated, err := dc.Resource(KustomizationGVR).Namespace(req.Namespace).Update(ctx, obj, metav1.UpdateOptions{
		FieldManager: "kprompt",
	})
	if err != nil {
		return SyncResult{}, fmt.Errorf("reconcile kustomization: %w", err)
	}
	return SyncResult{
		Engine:    "flux",
		Kind:      FluxKind,
		Name:      updated.GetName(),
		Namespace: updated.GetNamespace(),
		Action:    req.Action,
		Message:   fmt.Sprintf("requested Flux reconcile (%s)", req.Action),
	}, nil
}

func syncArgoCD(ctx context.Context, dc dynamic.Interface, req SyncRequest) (SyncResult, error) {
	obj, err := dc.Resource(ApplicationGVR).Namespace(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return SyncResult{}, fmt.Errorf("get application: %w", err)
	}

	revision := strings.TrimSpace(req.Revision)
	if revision == "" {
		revision = "HEAD"
	}
	if req.Action == "rollback" {
		if prev, ok, _ := unstructured.NestedString(obj.Object, "status", "history"); !ok {
			_ = prev
		}
		// Prefer explicit revision; otherwise keep HEAD and note rollback intent.
		if req.Revision == "" {
			if hist, ok, _ := unstructured.NestedSlice(obj.Object, "status", "history"); ok && len(hist) > 0 {
				if last, ok := hist[len(hist)-1].(map[string]any); ok {
					if rev, ok := last["revision"].(string); ok && rev != "" {
						revision = rev
					}
				}
				if len(hist) >= 2 {
					if prev, ok := hist[len(hist)-2].(map[string]any); ok {
						if rev, ok := prev["revision"].(string); ok && rev != "" {
							revision = rev
						}
					}
				}
			}
		}
	}

	op := map[string]any{
		"initiatedBy": map[string]any{"username": "kprompt"},
		"sync": map[string]any{
			"revision": revision,
		},
	}
	if err := unstructured.SetNestedMap(obj.Object, op, "operation"); err != nil {
		return SyncResult{}, fmt.Errorf("set operation: %w", err)
	}
	anns := obj.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns["argocd.argoproj.io/refresh"] = "hard"
	obj.SetAnnotations(anns)

	updated, err := dc.Resource(ApplicationGVR).Namespace(req.Namespace).Update(ctx, obj, metav1.UpdateOptions{
		FieldManager: "kprompt",
	})
	if err != nil {
		// Fallback to merge-patch style annotation-only refresh if full update fails on operation.
		patch := fmt.Sprintf(
			`{"metadata":{"annotations":{"argocd.argoproj.io/refresh":"hard"}},"operation":{"initiatedBy":{"username":"kprompt"},"sync":{"revision":%q}}}`,
			revision,
		)
		updated, err = dc.Resource(ApplicationGVR).Namespace(req.Namespace).Patch(
			ctx, req.Name, types.MergePatchType, []byte(patch), metav1.PatchOptions{FieldManager: "kprompt"},
		)
		if err != nil {
			return SyncResult{}, fmt.Errorf("sync application: %w", err)
		}
	}
	return SyncResult{
		Engine:    "argocd",
		Kind:      ArgoCDKind,
		Name:      updated.GetName(),
		Namespace: updated.GetNamespace(),
		Action:    req.Action,
		Message:   fmt.Sprintf("requested Argo CD sync (%s) revision=%s", req.Action, revision),
	}, nil
}

// Label formats a human-readable sync result line.
func (s SyncResult) Label() string {
	return fmt.Sprintf("%s %s/%s — %s", s.Engine, s.Kind, s.Name, s.Message)
}
