package suggest

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/optimize"
	"github.com/kprompt/kprompt/internal/planner"
)

const maxOptimizeSuggestions = 5

// FromOptimize maps top optimize findings to optional scale/patch follow-ups (T-057).
// Plans always require approval; callers must not auto-apply from the parent optimize --approve flag.
func FromOptimize(ctx context.Context, client kubernetes.Interface, rep optimize.Report) ([]Suggestion, error) {
	if client == nil {
		return nil, nil
	}
	var out []Suggestion
	seen := map[string]bool{}

	// Rightsizing → Deployment resource patches (executor supports Deployments only).
	type key struct{ ns, kind, name string }
	deltasByWL := map[key][]optimize.RightsizingDelta{}
	for _, d := range rep.Rightsizing {
		k := key{ns: d.Namespace, kind: d.Kind, name: d.Name}
		deltasByWL[k] = append(deltasByWL[k], d)
	}
	keys := make([]key, 0, len(deltasByWL))
	for k := range deltasByWL {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].ns != keys[j].ns {
			return keys[i].ns < keys[j].ns
		}
		if keys[i].kind != keys[j].kind {
			return keys[i].kind < keys[j].kind
		}
		return keys[i].name < keys[j].name
	})
	for _, k := range keys {
		deltas := deltasByWL[k]
		if len(out) >= maxOptimizeSuggestions {
			break
		}
		sk := k.kind + "|" + k.ns + "|" + k.name + "|patch"
		if seen[sk] {
			continue
		}
		seen[sk] = true
		if k.kind != optimize.WorkloadDeployment {
			out = append(out, Suggestion{
				Code:    "optimize.rightsizing",
				Title:   fmt.Sprintf("Review resources on %s/%s", k.kind, k.name),
				Prompt:  fmt.Sprintf("patch resources on %s/%s", k.kind, k.name),
				Summary: "StatefulSet resource patches are prompt-only in this release; apply manually or convert to a Deployment plan",
			})
			continue
		}
		s, err := suggestRightsizingPatch(ctx, client, k.ns, k.name, deltas)
		if err != nil {
			return out, err
		}
		if s != nil {
			out = append(out, *s)
		}
	}

	// HPA: scale when desired > current; maxed / static → prompt-only.
	for _, h := range rep.HPA {
		if len(out) >= maxOptimizeSuggestions {
			break
		}
		sk := h.Kind + "|" + h.Namespace + "|" + h.Name + "|hpa"
		if seen[sk] {
			continue
		}
		seen[sk] = true

		if h.Maxed {
			out = append(out, Suggestion{
				Code:   "optimize.hpa.maxed",
				Title:  fmt.Sprintf("Raise HPA max for %s/%s", h.Kind, h.Name),
				Prompt: fmt.Sprintf("raise HPA max for %s", h.Name),
				Summary: fmt.Sprintf(
					"HPA is at max replicas — raise the HPA max (not auto-planned); current hint: %s",
					h.Message,
				),
			})
			continue
		}
		if h.StaticReplicas {
			out = append(out, Suggestion{
				Code:    "optimize.hpa.static",
				Title:   fmt.Sprintf("Add HPA for %s/%s", h.Kind, h.Name),
				Prompt:  fmt.Sprintf("add HPA for %s", h.Name),
				Summary: h.Message,
			})
			continue
		}
		if h.HasHPA && h.Desired != nil && h.Current != nil && *h.Desired > *h.Current &&
			h.Kind == optimize.WorkloadDeployment {
			s := suggestScale(h.Namespace, h.Name, *h.Desired, h.Message)
			out = append(out, s)
		}
	}

	return out, nil
}

func suggestScale(namespace, name string, replicas int32, reason string) Suggestion {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	rep := replicas
	plan := &planner.ExecutionPlan{
		Intent: intent.Intent{
			Kind: intent.KindScale,
			Target: intent.Target{
				Name:      name,
				Namespace: ns,
				Kind:      "Deployment",
			},
			Params: map[string]any{
				"replicas": int(replicas),
				"reason":   "optimize.hpa",
			},
		},
		Actions: []planner.Action{{
			Op: planner.OpScale,
			Object: planner.ObjectRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       name,
				Namespace:  ns,
			},
			Replicas: &rep,
			Diff:     fmt.Sprintf("scale Deployment/%s to %d replicas", name, replicas),
		}},
		Summary:          fmt.Sprintf("Scale Deployment/%s to %d replicas (%s)", name, replicas, reason),
		RequiresApproval: true,
	}
	return Suggestion{
		Code:    "optimize.hpa.scale",
		Title:   fmt.Sprintf("Scale %s to %d", name, replicas),
		Prompt:  fmt.Sprintf("scale %s to %d", name, replicas),
		Plan:    plan,
		Summary: plan.Summary,
	}
}

func suggestRightsizingPatch(
	ctx context.Context,
	client kubernetes.Interface,
	namespace, name string,
	deltas []optimize.RightsizingDelta,
) (*Suggestion, error) {
	ns := strings.TrimSpace(namespace)
	if ns == "" {
		ns = "default"
	}
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return &Suggestion{
			Code:    "optimize.rightsizing",
			Title:   fmt.Sprintf("Patch resources on %s", name),
			Prompt:  fmt.Sprintf("patch resources on %s", name),
			Summary: fmt.Sprintf("Could not load Deployment/%s for an auto-plan: %v", name, err),
		}, nil
	}
	if len(dep.Spec.Template.Spec.Containers) == 0 {
		return nil, nil
	}

	patched := dep.DeepCopy()
	c := &patched.Spec.Template.Spec.Containers[0]
	if c.Resources.Requests == nil {
		c.Resources.Requests = corev1.ResourceList{}
	}
	if c.Resources.Limits == nil {
		c.Resources.Limits = corev1.ResourceList{}
	}

	var parts []string
	for _, d := range deltas {
		q, err := resource.ParseQuantity(d.Suggested)
		if err != nil {
			continue
		}
		switch d.Resource + "/" + d.Field {
		case "cpu/request":
			c.Resources.Requests[corev1.ResourceCPU] = q
		case "cpu/limit":
			c.Resources.Limits[corev1.ResourceCPU] = q
		case "memory/request":
			c.Resources.Requests[corev1.ResourceMemory] = q
		case "memory/limit":
			c.Resources.Limits[corev1.ResourceMemory] = q
		default:
			continue
		}
		parts = append(parts, fmt.Sprintf("%s %s %s→%s", d.Direction, d.Resource+" "+d.Field, d.Current, d.Suggested))
	}
	if len(parts) == 0 {
		return nil, nil
	}

	patched.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}
	// Clear status for cleaner apply manifests.
	patched.Status = appsv1.DeploymentStatus{}
	raw, err := yaml.Marshal(patched)
	if err != nil {
		return nil, err
	}

	diff := fmt.Sprintf("~ Deployment/%s (update)\n  container: %s\n  %s",
		dep.Name, c.Name, strings.Join(parts, "\n  "))
	plan := &planner.ExecutionPlan{
		Intent: intent.Intent{
			Kind: intent.KindPatch,
			Target: intent.Target{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
			},
			Params: map[string]any{
				"reason":     "optimize.rightsizing",
				"container":  c.Name,
				"changes":    len(parts),
			},
		},
		Actions: []planner.Action{{
			Op: planner.OpUpdate,
			Object: planner.ObjectRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       dep.Name,
				Namespace:  dep.Namespace,
			},
			Manifest: string(raw),
			Diff:     diff,
		}},
		Summary:          fmt.Sprintf("Rightsizing patch on Deployment/%s: %s", dep.Name, strings.Join(parts, "; ")),
		RequiresApproval: true,
	}
	return &Suggestion{
		Code:    "optimize.rightsizing",
		Title:   fmt.Sprintf("Patch resources on %s", dep.Name),
		Prompt:  fmt.Sprintf("patch resources on %s", dep.Name),
		Plan:    plan,
		Summary: plan.Summary,
	}, nil
}
