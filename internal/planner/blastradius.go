package planner

import (
	"context"
	"fmt"
	"sort"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
)

const (
	maxBlastRelatedPerKind = 12
	maxBlastRelatedTotal   = 24
	maxBlastLabelKeys      = 16
)

// BlastRadius is a review-aid summary of namespaces and related objects
// a mutating plan may affect. Not a dashboard or full dependency graph.
type BlastRadius struct {
	Namespaces []string      `json:"namespaces"`
	Targets    []BlastTarget `json:"targets,omitempty"`
	Notes      []string      `json:"notes,omitempty"`
}

// BlastTarget describes one planned mutate and nearby cluster objects.
type BlastTarget struct {
	Op        string            `json:"op"`
	Kind      string            `json:"kind"`
	Name      string            `json:"name"`
	Namespace string            `json:"namespace,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Owners    []string          `json:"owners,omitempty"`
	Related   []BlastRelated    `json:"related,omitempty"`
	NotFound  bool              `json:"notFound,omitempty"`
}

// BlastRelated is an HPA, NetworkPolicy, Service, or similar neighbor.
type BlastRelated struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace,omitempty"`
	Relation  string `json:"relation"`
}

// EnrichBlastRadius fills plan.BlastRadius for mutating Kubernetes actions.
// Failures are ignored so plan printing still works.
func EnrichBlastRadius(ctx context.Context, client kubernetes.Interface, plan *ExecutionPlan) {
	if client == nil || plan == nil || !plan.RequiresApproval {
		return
	}
	nsSet := map[string]struct{}{}
	var targets []BlastTarget
	var notes []string

	for i := range plan.Actions {
		a := plan.Actions[i]
		if !isBlastMutateOp(a.Op) {
			continue
		}
		if a.Object.Namespace != "" {
			nsSet[a.Object.Namespace] = struct{}{}
		} else if a.Object.Kind != "" {
			nsSet["default"] = struct{}{}
		}
		t, extra := blastForAction(ctx, client, a)
		if t != nil {
			targets = append(targets, *t)
		}
		notes = append(notes, extra...)
	}

	if len(targets) == 0 && len(nsSet) == 0 {
		return
	}
	plan.BlastRadius = &BlastRadius{
		Namespaces: sortedKeys(nsSet),
		Targets:    targets,
		Notes:      uniqueStrings(notes),
	}
}

func isBlastMutateOp(op Op) bool {
	switch op {
	case OpCreate, OpUpdate, OpScale, OpRollback, OpDelete:
		return true
	default:
		return false
	}
}

func blastForAction(ctx context.Context, client kubernetes.Interface, a Action) (*BlastTarget, []string) {
	ns := namespaceOrDefault(a.Object.Namespace)
	t := &BlastTarget{
		Op:        string(a.Op),
		Kind:      a.Object.Kind,
		Name:      a.Object.Name,
		Namespace: ns,
	}
	var notes []string

	switch strings.ToLower(a.Object.Kind) {
	case "deployment":
		notes = append(notes, blastDeployment(ctx, client, ns, a, t)...)
	case "service":
		notes = append(notes, blastService(ctx, client, ns, a, t)...)
	case "pod":
		notes = append(notes, blastPod(ctx, client, ns, a, t)...)
	default:
		if a.Object.Kind != "" {
			notes = append(notes, fmt.Sprintf("blast-radius detail limited for kind %s", a.Object.Kind))
		}
	}
	return t, notes
}

func blastDeployment(ctx context.Context, client kubernetes.Interface, ns string, a Action, t *BlastTarget) []string {
	var notes []string
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		t.NotFound = true
		if desired, derr := decodeDeployment(a.Manifest); derr == nil && desired != nil {
			t.Labels = clipLabels(desired.Labels)
			var podLabels map[string]string
			if desired.Spec.Selector != nil {
				podLabels = desired.Spec.Selector.MatchLabels
			}
			t.Related = appendRelated(t.Related, findServicesForSelector(ctx, client, ns, podLabels)...)
			t.Related = appendRelated(t.Related, findNetworkPoliciesForLabels(ctx, client, ns, podLabels)...)
		}
		t.Related = appendRelated(t.Related, findHPAsForTarget(ctx, client, ns, "Deployment", a.Object.Name)...)
		t.Related = clipRelatedTotal(t.Related)
		return notes
	}
	if err != nil {
		notes = append(notes, fmt.Sprintf("could not load Deployment/%s: %v", a.Object.Name, err))
		return notes
	}
	t.Labels = clipLabels(dep.Labels)
	t.Owners = ownerRefStrings(dep.OwnerReferences)
	podLabels := map[string]string{}
	if dep.Spec.Selector != nil {
		podLabels = dep.Spec.Selector.MatchLabels
	}
	if len(podLabels) == 0 {
		podLabels = dep.Spec.Template.Labels
	}
	t.Related = appendRelated(t.Related, findHPAsForTarget(ctx, client, ns, "Deployment", dep.Name)...)
	t.Related = appendRelated(t.Related, findServicesForSelector(ctx, client, ns, podLabels)...)
	t.Related = appendRelated(t.Related, findNetworkPoliciesForLabels(ctx, client, ns, podLabels)...)
	t.Related = appendRelated(t.Related, findReplicaSetsForDeployment(ctx, client, ns, dep)...)
	t.Related = clipRelatedTotal(t.Related)
	return notes
}

func blastService(ctx context.Context, client kubernetes.Interface, ns string, a Action, t *BlastTarget) []string {
	var notes []string
	svc, err := client.CoreV1().Services(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		t.NotFound = true
		if desired, derr := decodeService(a.Manifest); derr == nil && desired != nil {
			t.Labels = clipLabels(desired.Labels)
			t.Related = appendRelated(t.Related, findNetworkPoliciesForLabels(ctx, client, ns, desired.Spec.Selector)...)
		}
		return notes
	}
	if err != nil {
		notes = append(notes, fmt.Sprintf("could not load Service/%s: %v", a.Object.Name, err))
		return notes
	}
	t.Labels = clipLabels(svc.Labels)
	t.Owners = ownerRefStrings(svc.OwnerReferences)
	t.Related = appendRelated(t.Related, findNetworkPoliciesForLabels(ctx, client, ns, svc.Spec.Selector)...)
	return notes
}

func blastPod(ctx context.Context, client kubernetes.Interface, ns string, a Action, t *BlastTarget) []string {
	var notes []string
	pod, err := client.CoreV1().Pods(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		t.NotFound = true
		return notes
	}
	if err != nil {
		notes = append(notes, fmt.Sprintf("could not load Pod/%s: %v", a.Object.Name, err))
		return notes
	}
	t.Labels = clipLabels(pod.Labels)
	t.Owners = ownerRefStrings(pod.OwnerReferences)
	t.Related = appendRelated(t.Related, findNetworkPoliciesForLabels(ctx, client, ns, pod.Labels)...)
	t.Related = appendRelated(t.Related, findServicesForSelector(ctx, client, ns, pod.Labels)...)
	return notes
}

func findHPAsForTarget(ctx context.Context, client kubernetes.Interface, ns, kind, name string) []BlastRelated {
	var out []BlastRelated
	list, err := client.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, h := range list.Items {
			if hpaTargets(h.Spec.ScaleTargetRef.Kind, h.Spec.ScaleTargetRef.Name, kind, name) {
				out = append(out, BlastRelated{
					Kind: "HorizontalPodAutoscaler", Name: h.Name, Namespace: ns, Relation: "scales",
				})
			}
		}
		return clipRelated(out)
	}
	v1list, v1err := client.AutoscalingV1().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if v1err != nil {
		return nil
	}
	for _, h := range v1list.Items {
		if hpaTargets(h.Spec.ScaleTargetRef.Kind, h.Spec.ScaleTargetRef.Name, kind, name) {
			out = append(out, BlastRelated{
				Kind: "HorizontalPodAutoscaler", Name: h.Name, Namespace: ns, Relation: "scales",
			})
		}
	}
	return clipRelated(out)
}

func hpaTargets(refKind, refName, wantKind, wantName string) bool {
	if !strings.EqualFold(refName, wantName) {
		return false
	}
	return strings.EqualFold(refKind, wantKind)
}

func findServicesForSelector(ctx context.Context, client kubernetes.Interface, ns string, podLabels map[string]string) []BlastRelated {
	if len(podLabels) == 0 {
		return nil
	}
	list, err := client.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	set := labels.Set(podLabels)
	var out []BlastRelated
	for _, svc := range list.Items {
		if len(svc.Spec.Selector) == 0 {
			continue
		}
		sel := labels.SelectorFromSet(svc.Spec.Selector)
		if sel.Matches(set) {
			out = append(out, BlastRelated{
				Kind: "Service", Name: svc.Name, Namespace: ns, Relation: "routes-to",
			})
		}
	}
	return clipRelated(out)
}

func findNetworkPoliciesForLabels(ctx context.Context, client kubernetes.Interface, ns string, podLabels map[string]string) []BlastRelated {
	list, err := client.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	var out []BlastRelated
	for _, np := range list.Items {
		if networkPolicySelectsLabels(np, podLabels) {
			out = append(out, BlastRelated{
				Kind: "NetworkPolicy", Name: np.Name, Namespace: ns, Relation: "selects-pods",
			})
		}
	}
	return clipRelated(out)
}

func networkPolicySelectsLabels(np networkingv1.NetworkPolicy, podLabels map[string]string) bool {
	sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
	if err != nil {
		return false
	}
	if sel.Empty() {
		return true
	}
	if len(podLabels) == 0 {
		return false
	}
	return sel.Matches(labels.Set(podLabels))
}

func findReplicaSetsForDeployment(ctx context.Context, client kubernetes.Interface, ns string, dep *appsv1.Deployment) []BlastRelated {
	list, err := client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil
	}
	var out []BlastRelated
	for _, rs := range list.Items {
		if !ownedBy(rs.OwnerReferences, "Deployment", dep.Name, dep.UID) {
			continue
		}
		out = append(out, BlastRelated{
			Kind: "ReplicaSet", Name: rs.Name, Namespace: ns, Relation: "owned",
		})
	}
	return clipRelated(out)
}

func ownedBy(refs []metav1.OwnerReference, kind, name string, uid types.UID) bool {
	for _, o := range refs {
		if o.Kind == kind && o.Name == name && (uid == "" || o.UID == uid) {
			return true
		}
	}
	return false
}

func ownerRefStrings(refs []metav1.OwnerReference) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, r := range refs {
		out = append(out, fmt.Sprintf("%s/%s", r.Kind, r.Name))
	}
	return out
}

func clipLabels(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	keys := make([]string, 0, len(in))
	for k := range in {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > maxBlastLabelKeys {
		keys = keys[:maxBlastLabelKeys]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = in[k]
	}
	return out
}

func appendRelated(dst []BlastRelated, more ...BlastRelated) []BlastRelated {
	return append(dst, more...)
}

func clipRelated(in []BlastRelated) []BlastRelated {
	if len(in) <= maxBlastRelatedPerKind {
		return in
	}
	return in[:maxBlastRelatedPerKind]
}

func clipRelatedTotal(in []BlastRelated) []BlastRelated {
	if len(in) <= maxBlastRelatedTotal {
		return in
	}
	return in[:maxBlastRelatedTotal]
}

func sortedKeys(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, s := range in {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}
