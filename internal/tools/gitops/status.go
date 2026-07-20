package gitops

import (
	"context"
	"fmt"
	"sort"
	"strings"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"github.com/kprompt/kprompt/internal/cluster"
)

var (
	KustomizationGVR = schema.GroupVersionResource{
		Group: FluxGroup, Version: "v1", Resource: "kustomizations",
	}
	ApplicationGVR = schema.GroupVersionResource{
		Group: ArgoCDGroup, Version: "v1alpha1", Resource: "applications",
	}
)

// StatusRequest configures a read-only GitOps sync/health summary.
type StatusRequest struct {
	Namespace string
	Name      string
	Engine    string // flux | argocd | auto
}

// AppStatus is one Flux Kustomization or Argo CD Application.
type AppStatus struct {
	Engine    string `json:"engine"` // flux | argocd
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Sync      string `json:"sync,omitempty"`
	Health    string `json:"health,omitempty"`
	Revision  string `json:"revision,omitempty"`
	Message   string `json:"message,omitempty"`
}

// StatusReport is the stable human + JSON contract for GitOps status (T-043).
type StatusReport struct {
	Type      string      `json:"type"`
	Scope     string      `json:"scope"`
	Namespace string      `json:"namespace,omitempty"`
	Summary   string      `json:"summary"`
	Apps      []AppStatus `json:"apps"`
	Notes     []string    `json:"notes,omitempty"`
}

// SummarizeStatus lists Flux Kustomizations and/or Argo CD Applications with sync/health.
func SummarizeStatus(ctx context.Context, cfg *rest.Config, req StatusRequest) (StatusReport, error) {
	if cfg == nil {
		return StatusReport{}, fmt.Errorf("gitops status: rest config is nil")
	}
	dc, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return StatusReport{}, fmt.Errorf("gitops dynamic client: %w", err)
	}
	av, err := Detect(ctx, cfg)
	if err != nil {
		return StatusReport{}, err
	}
	return SummarizeStatusWithClient(ctx, dc, av, req)
}

// SummarizeStatusWithClient builds a status report using an injected client.
func SummarizeStatusWithClient(ctx context.Context, dc dynamic.Interface, av Availability, req StatusRequest) (StatusReport, error) {
	ns := strings.TrimSpace(req.Namespace)
	name := strings.TrimSpace(req.Name)
	engine := strings.ToLower(strings.TrimSpace(req.Engine))
	scope := "cluster"
	if ns != "" {
		scope = "namespace"
	}
	rep := StatusReport{
		Type:      "gitops-status",
		Scope:     scope,
		Namespace: ns,
		Apps:      make([]AppStatus, 0, 8),
	}
	if !av.Installed {
		rep.Notes = append(rep.Notes, "no Flux or Argo CD CRDs detected")
		rep.Summary = "GitOps controllers not available"
		return rep, nil
	}

	wantFlux := engine == "" || engine == "auto" || engine == "flux"
	wantArgo := engine == "" || engine == "auto" || engine == "argocd" || engine == "argo"

	if wantFlux && av.Flux {
		apps, notes, err := listOrGet(ctx, dc, KustomizationGVR, ns, name, summarizeFlux)
		if err != nil {
			return StatusReport{}, err
		}
		rep.Apps = append(rep.Apps, apps...)
		rep.Notes = append(rep.Notes, notes...)
	} else if wantFlux && !av.Flux {
		rep.Notes = append(rep.Notes, "Flux Kustomization CRD not installed")
	}

	if wantArgo && av.ArgoCD {
		apps, notes, err := listOrGet(ctx, dc, ApplicationGVR, ns, name, summarizeArgoCD)
		if err != nil {
			return StatusReport{}, err
		}
		rep.Apps = append(rep.Apps, apps...)
		rep.Notes = append(rep.Notes, notes...)
	} else if wantArgo && !av.ArgoCD {
		rep.Notes = append(rep.Notes, "Argo CD Application CRD not installed")
	}

	sort.Slice(rep.Apps, func(i, j int) bool {
		if rep.Apps[i].Engine != rep.Apps[j].Engine {
			return rep.Apps[i].Engine < rep.Apps[j].Engine
		}
		if rep.Apps[i].Namespace != rep.Apps[j].Namespace {
			return rep.Apps[i].Namespace < rep.Apps[j].Namespace
		}
		return rep.Apps[i].Name < rep.Apps[j].Name
	})

	healthy, degraded, outOfSync := 0, 0, 0
	for _, a := range rep.Apps {
		if strings.EqualFold(a.Health, "Healthy") || strings.EqualFold(a.Health, "True") {
			healthy++
		} else if a.Health != "" {
			degraded++
		}
		if strings.EqualFold(a.Sync, "OutOfSync") || strings.EqualFold(a.Sync, "False") {
			outOfSync++
		}
	}
	switch {
	case len(rep.Apps) == 0:
		rep.Summary = "No GitOps apps found"
	default:
		rep.Summary = fmt.Sprintf("%d app(s): %d healthy, %d degraded/other, %d out-of-sync",
			len(rep.Apps), healthy, degraded, outOfSync)
	}
	return rep, nil
}

func listOrGet(
	ctx context.Context,
	dc dynamic.Interface,
	gvr schema.GroupVersionResource,
	ns, name string,
	summarize func(*unstructured.Unstructured) AppStatus,
) ([]AppStatus, []string, error) {
	limit := int64(cluster.DefaultReadLimit)
	var notes []string
	var objs []unstructured.Unstructured

	if name != "" {
		if ns == "" {
			return nil, nil, fmt.Errorf("%s %q requires a namespace", gvr.Resource, name)
		}
		obj, err := dc.Resource(gvr).Namespace(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			if apierrors.IsNotFound(err) {
				notes = append(notes, fmt.Sprintf("%s/%s not found in %s", gvr.Resource, name, ns))
				return nil, notes, nil
			}
			return nil, nil, err
		}
		objs = append(objs, *obj)
	} else {
		var list *unstructured.UnstructuredList
		var err error
		if ns == "" {
			list, err = dc.Resource(gvr).List(ctx, metav1.ListOptions{Limit: limit})
		} else {
			list, err = dc.Resource(gvr).Namespace(ns).List(ctx, metav1.ListOptions{Limit: limit})
		}
		if err != nil {
			if apierrors.IsForbidden(err) {
				notes = append(notes, fmt.Sprintf("skipped %s: %v", gvr.Resource, err))
				return nil, notes, nil
			}
			return nil, nil, fmt.Errorf("list %s: %w", gvr.Resource, err)
		}
		objs = list.Items
	}

	out := make([]AppStatus, 0, len(objs))
	for i := range objs {
		out = append(out, summarize(&objs[i]))
	}
	return out, notes, nil
}

func summarizeFlux(obj *unstructured.Unstructured) AppStatus {
	st := AppStatus{
		Engine:    "flux",
		Kind:      FluxKind,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if rev, ok, _ := unstructured.NestedString(obj.Object, "status", "lastAppliedRevision"); ok {
		st.Revision = rev
	} else if rev, ok, _ := unstructured.NestedString(obj.Object, "status", "lastAttemptedRevision"); ok {
		st.Revision = rev
	}
	conds, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		for _, raw := range conds {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			typ, _ := m["type"].(string)
			if !strings.EqualFold(typ, "Ready") {
				continue
			}
			status, _ := m["status"].(string)
			msg, _ := m["message"].(string)
			st.Health = strings.TrimSpace(status)
			st.Sync = strings.TrimSpace(status)
			st.Message = strings.TrimSpace(msg)
			if strings.EqualFold(status, "True") {
				st.Health = "Healthy"
				st.Sync = "Synced"
			} else if strings.EqualFold(status, "False") {
				st.Health = "Degraded"
				st.Sync = "OutOfSync"
			}
			break
		}
	}
	return st
}

func summarizeArgoCD(obj *unstructured.Unstructured) AppStatus {
	st := AppStatus{
		Engine:    "argocd",
		Kind:      ArgoCDKind,
		Name:      obj.GetName(),
		Namespace: obj.GetNamespace(),
	}
	if sync, ok, _ := unstructured.NestedString(obj.Object, "status", "sync", "status"); ok {
		st.Sync = sync
	}
	if health, ok, _ := unstructured.NestedString(obj.Object, "status", "health", "status"); ok {
		st.Health = health
	}
	if rev, ok, _ := unstructured.NestedString(obj.Object, "status", "sync", "revision"); ok {
		st.Revision = rev
	}
	if msg, ok, _ := unstructured.NestedString(obj.Object, "status", "conditions"); !ok {
		_ = msg
	}
	conds, found, _ := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if found {
		for _, raw := range conds {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			msg, _ := m["message"].(string)
			if strings.TrimSpace(msg) != "" {
				st.Message = strings.TrimSpace(msg)
				break
			}
		}
	}
	return st
}
