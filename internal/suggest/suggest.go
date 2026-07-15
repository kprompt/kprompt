package suggest

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

// Suggestion is a follow-up action derived from explain findings.
type Suggestion struct {
	Code    string
	Title   string
	Prompt  string // copy-pasteable follow-up prompt
	Plan    *planner.ExecutionPlan
	Summary string
}

// FromExplain maps explain findings to prompts and optional mutation plans.
// Actionable plans (e.g. OOM → raise memory) still require approval to apply.
func FromExplain(ctx context.Context, client kubernetes.Interface, rep cluster.ExplainReport) ([]Suggestion, error) {
	if client == nil {
		return nil, nil
	}
	var out []Suggestion
	seen := map[string]bool{}
	for _, f := range rep.Findings {
		key := f.Code + "|" + f.Container
		if seen[key] {
			continue
		}
		seen[key] = true
		switch f.Code {
		case "OOMKilled":
			s, err := suggestOOM(ctx, client, rep, f)
			if err != nil {
				return out, err
			}
			if s != nil {
				out = append(out, *s)
			}
		case "CrashLoopBackOff":
			out = append(out, Suggestion{
				Code:    f.Code,
				Title:   "Inspect crash logs",
				Prompt:  fmt.Sprintf(`logs %s`, rep.Target),
				Summary: "Fetch recent container logs to see why the process exits",
			})
		case "ImagePullBackOff", "ErrImagePull":
			out = append(out, Suggestion{
				Code:    f.Code,
				Title:   "Check image name / pull secrets",
				Prompt:  fmt.Sprintf(`describe %s`, rep.Target),
				Summary: "Verify the image reference and registry credentials",
			})
		}
	}
	return out, nil
}

func suggestOOM(ctx context.Context, client kubernetes.Interface, rep cluster.ExplainReport, f cluster.Finding) (*Suggestion, error) {
	dep, container, err := resolveDeploymentContainer(ctx, client, rep, f.Container)
	if err != nil || dep == nil {
		return &Suggestion{
			Code:    "OOMKilled",
			Title:   "Raise memory limit",
			Prompt:  fmt.Sprintf(`raise memory for %s`, rep.Target),
			Summary: "Could not load Deployment for an auto-plan; try a manual resource patch",
		}, nil
	}
	idx := containerIndex(dep, container)
	if idx < 0 {
		idx = 0
		container = dep.Spec.Template.Spec.Containers[0].Name
	}
	oldLimit, newLimit := bumpMemory(dep.Spec.Template.Spec.Containers[idx].Resources.Limits)
	oldReq, newReq := bumpMemory(dep.Spec.Template.Spec.Containers[idx].Resources.Requests)

	patched := dep.DeepCopy()
	c := &patched.Spec.Template.Spec.Containers[idx]
	if c.Resources.Limits == nil {
		c.Resources.Limits = corev1.ResourceList{}
	}
	if c.Resources.Requests == nil {
		c.Resources.Requests = corev1.ResourceList{}
	}
	c.Resources.Limits[corev1.ResourceMemory] = newLimit
	if !oldReq.IsZero() || !dep.Spec.Template.Spec.Containers[idx].Resources.Requests.Memory().IsZero() {
		c.Resources.Requests[corev1.ResourceMemory] = newReq
	} else {
		req := newLimit.DeepCopy()
		if v := newLimit.Value(); v > 0 {
			req.Set(v / 2)
		}
		c.Resources.Requests[corev1.ResourceMemory] = req
	}
	patched.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"}
	raw, err := yaml.Marshal(patched)
	if err != nil {
		return nil, err
	}

	diff := fmt.Sprintf("~ Deployment/%s (update)\n  container: %s\n  memory limit: %s → %s",
		dep.Name, container, qtyString(oldLimit), qtyString(newLimit))
	plan := &planner.ExecutionPlan{
		Intent: intent.Intent{
			Kind: intent.KindPatch,
			Target: intent.Target{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
			},
			Params: map[string]any{
				"reason":    "OOMKilled",
				"container": container,
				"memory":    newLimit.String(),
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
		Summary:          fmt.Sprintf("Raise memory limit on Deployment/%s container %s (%s → %s)", dep.Name, container, qtyString(oldLimit), qtyString(newLimit)),
		RequiresApproval: true,
	}
	return &Suggestion{
		Code:    "OOMKilled",
		Title:   "Raise memory limit",
		Prompt:  fmt.Sprintf(`raise memory for %s to %s`, dep.Name, newLimit.String()),
		Plan:    plan,
		Summary: plan.Summary,
	}, nil
}

func resolveDeploymentContainer(ctx context.Context, client kubernetes.Interface, rep cluster.ExplainReport, container string) (*appsv1.Deployment, string, error) {
	ns := rep.Namespace
	if ns == "" {
		ns = "default"
	}
	name := rep.Target
	if rep.Kind == "Deployment" || rep.Kind == "" {
		dep, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err == nil {
			return dep, container, nil
		}
	}
	pod, err := client.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, container, err
	}
	for _, ow := range pod.OwnerReferences {
		if ow.Kind == "ReplicaSet" && ow.Controller != nil && *ow.Controller {
			rs, err := client.AppsV1().ReplicaSets(ns).Get(ctx, ow.Name, metav1.GetOptions{})
			if err != nil {
				return nil, container, err
			}
			for _, row := range rs.OwnerReferences {
				if row.Kind == "Deployment" && row.Controller != nil && *row.Controller {
					dep, err := client.AppsV1().Deployments(ns).Get(ctx, row.Name, metav1.GetOptions{})
					return dep, container, err
				}
			}
		}
	}
	return nil, container, fmt.Errorf("no owning Deployment for Pod/%s", name)
}

func containerIndex(dep *appsv1.Deployment, name string) int {
	if name == "" {
		return 0
	}
	for i, c := range dep.Spec.Template.Spec.Containers {
		if c.Name == name {
			return i
		}
	}
	return -1
}

func bumpMemory(list corev1.ResourceList) (oldQty, newQty resource.Quantity) {
	if list != nil {
		if q, ok := list[corev1.ResourceMemory]; ok && !q.IsZero() {
			oldQty = q.DeepCopy()
			v := q.Value()
			if v <= 0 {
				newQty = resource.MustParse("256Mi")
				return oldQty, newQty
			}
			newQty = *resource.NewQuantity(v*2, resource.BinarySI)
			return oldQty, newQty
		}
	}
	newQty = resource.MustParse("256Mi")
	return oldQty, newQty
}

func qtyString(q resource.Quantity) string {
	if q.IsZero() {
		return "(none)"
	}
	return q.String()
}

// ActionablePlans returns suggestions that carry an ExecutionPlan.
func ActionablePlans(suggestions []Suggestion) []Suggestion {
	var out []Suggestion
	for _, s := range suggestions {
		if s.Plan != nil {
			out = append(out, s)
		}
	}
	return out
}

// FormatPromptHint returns a shell-friendly follow-up example.
func FormatPromptHint(s Suggestion) string {
	p := strings.TrimSpace(s.Prompt)
	if p == "" {
		return ""
	}
	return fmt.Sprintf(`kprompt "%s" --approve`, p)
}
