package suggest

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"

	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

// FromAudit turns hygiene findings into one aggregate, approve-gated harden plan
// plus guidance for findings kprompt will not auto-patch.
//
// The only auto-patched fixes REMOVE a privilege grant (privileged=false,
// allowPrivilegeEscalation=false) on Deployment / StatefulSet / DaemonSet
// containers — changes that never invent workload-specific values and never
// tighten a constraint that could stop a container from starting. Everything else
// (runAsNonRoot, host namespaces, image tags, resource requests/limits) stays
// guidance-only.
func FromAudit(ctx context.Context, client kubernetes.Interface, inv incident.Investigation) ([]Suggestion, error) {
	if client == nil {
		return nil, nil
	}

	type workloadKey struct{ kind, name, ns string }
	patchable := map[workloadKey][]incident.Finding{}
	var order []workloadKey
	seenGuidance := map[string]bool{}
	var guidance []Suggestion

	for _, f := range inv.Findings {
		ref := auditResource(f)
		if ref == nil {
			continue
		}
		switch f.Code {
		case "Audit.Privileged", "Audit.PrivilegeEscalation":
			if !hardenableKind(ref.Kind) {
				addAuditGuidance(&guidance, seenGuidance, "Audit.UnsupportedKind",
					"Harden via workload manifest",
					fmt.Sprintf("describe %s/%s", strings.ToLower(ref.Kind), ref.Name),
					fmt.Sprintf("%s harden patches are not auto-generated — remove the privilege grant in your manifest", ref.Kind))
				continue
			}
			k := workloadKey{ref.Kind, ref.Name, ref.Namespace}
			if _, ok := patchable[k]; !ok {
				order = append(order, k)
			}
			patchable[k] = append(patchable[k], f)
		default:
			addAuditGuidance(&guidance, seenGuidance, f.Code, auditGuidanceTitle(f.Code), auditGuidancePrompt(f, ref), auditGuidanceSummary(f.Code))
		}
	}

	var actions []planner.Action
	for _, k := range order {
		act, err := hardenWorkload(ctx, client, k.kind, k.name, k.ns, patchable[k])
		if err != nil {
			guidance = append(guidance, Suggestion{
				Code:    "Audit.Harden",
				Title:   "Harden workload",
				Prompt:  fmt.Sprintf("harden %s", k.name),
				Summary: fmt.Sprintf("Could not build a patch for %s/%s: %v", k.kind, k.name, err),
			})
			continue
		}
		if act != nil {
			actions = append(actions, *act)
		}
	}

	var out []Suggestion
	if len(actions) > 0 {
		plan := &planner.ExecutionPlan{
			Intent: intent.Intent{
				Kind:   intent.KindPatch,
				Target: intent.Target{Kind: actions[0].Object.Kind, Namespace: inv.Namespace},
				Params: map[string]any{"reason": "AuditHarden"},
			},
			Actions:          actions,
			Summary:          fmt.Sprintf("Harden %d workload(s): remove privilege grants", len(actions)),
			RequiresApproval: true,
		}
		out = append(out, Suggestion{
			Code:    "Audit.Harden",
			Title:   "Remove privilege grants",
			Prompt:  "harden workloads",
			Plan:    plan,
			Summary: plan.Summary,
		})
	}
	out = append(out, guidance...)
	return out, nil
}

func hardenableKind(kind string) bool {
	switch kind {
	case "Deployment", "StatefulSet", "DaemonSet":
		return true
	default:
		return false
	}
}

// hardenWorkload returns a single OpUpdate action removing privilege grants from
// a Deployment / StatefulSet / DaemonSet, or nil when the live spec already has
// nothing to remove.
func hardenWorkload(ctx context.Context, client kubernetes.Interface, kind, name, ns string, findings []incident.Finding) (*planner.Action, error) {
	var (
		obj     any
		podSpec *corev1.PodSpec
	)
	switch kind {
	case "Deployment":
		dep, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		patched := dep.DeepCopy()
		patched.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: kind}
		obj, podSpec = patched, &patched.Spec.Template.Spec
	case "StatefulSet":
		sts, err := client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		patched := sts.DeepCopy()
		patched.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: kind}
		obj, podSpec = patched, &patched.Spec.Template.Spec
	case "DaemonSet":
		ds, err := client.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return nil, err
		}
		patched := ds.DeepCopy()
		patched.TypeMeta = metav1.TypeMeta{APIVersion: "apps/v1", Kind: kind}
		obj, podSpec = patched, &patched.Spec.Template.Spec
	default:
		return nil, fmt.Errorf("harden of %s not supported", kind)
	}

	changes := removePrivilegeGrants(podSpec, findings)
	if len(changes) == 0 {
		return nil, nil
	}
	raw, err := yaml.Marshal(obj)
	if err != nil {
		return nil, err
	}
	diff := fmt.Sprintf("~ %s/%s (update)\n  %s", kind, name, strings.Join(changes, "\n  "))
	return &planner.Action{
		Op: planner.OpUpdate,
		Object: planner.ObjectRef{
			APIVersion: "apps/v1",
			Kind:       kind,
			Name:       name,
			Namespace:  ns,
		},
		Manifest: string(raw),
		Diff:     diff,
	}, nil
}

// removePrivilegeGrants drops privileged / allowPrivilegeEscalation on the
// containers named by each finding, returning a human-readable change list.
func removePrivilegeGrants(spec *corev1.PodSpec, findings []incident.Finding) []string {
	var changes []string
	for _, f := range findings {
		container := containerFromMessage(f.Title)
		for _, c := range matchContainers(spec, container) {
			switch f.Code {
			case "Audit.Privileged":
				if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					c.SecurityContext.Privileged = boolRef(false)
					changes = append(changes, fmt.Sprintf("%s: privileged=false", c.Name))
				}
			case "Audit.PrivilegeEscalation":
				if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
					if c.SecurityContext == nil {
						c.SecurityContext = &corev1.SecurityContext{}
					}
					c.SecurityContext.AllowPrivilegeEscalation = boolRef(false)
					changes = append(changes, fmt.Sprintf("%s: allowPrivilegeEscalation=false", c.Name))
				}
			}
		}
	}
	return changes
}

// matchContainers returns pointers into the patched spec (init + main). When a
// container name is given it returns just that one; otherwise all containers so a
// privilege-removal applies broadly (always safe — it only drops a grant).
func matchContainers(spec *corev1.PodSpec, name string) []*corev1.Container {
	var all []*corev1.Container
	for i := range spec.InitContainers {
		all = append(all, &spec.InitContainers[i])
	}
	for i := range spec.Containers {
		all = append(all, &spec.Containers[i])
	}
	if name == "" {
		return all
	}
	for _, c := range all {
		if c.Name == name {
			return []*corev1.Container{c}
		}
	}
	return all
}

func auditResource(f incident.Finding) *incident.ResourceRef {
	for _, e := range f.Evidence {
		if e.Resource != nil && e.Resource.Name != "" {
			return e.Resource
		}
	}
	return nil
}

func addAuditGuidance(out *[]Suggestion, seen map[string]bool, code, title, prompt, summary string) {
	if seen[code] {
		return
	}
	seen[code] = true
	*out = append(*out, Suggestion{
		Code:    code,
		Title:   title,
		Prompt:  prompt,
		Summary: summary,
	})
}

func auditGuidanceTitle(code string) string {
	switch code {
	case "Audit.RunAsRoot":
		return "Run as non-root"
	case "Audit.HostNamespace":
		return "Drop host namespaces"
	case "Audit.LatestTag":
		return "Pin the image"
	case "Audit.MissingRequests":
		return "Set resource requests"
	case "Audit.MissingLimits":
		return "Set resource limits"
	case "Audit.MissingImagePullPolicy":
		return "Set imagePullPolicy"
	case "Audit.WritableRootFS":
		return "Set a read-only root filesystem"
	default:
		return "Review finding"
	}
}

func auditGuidancePrompt(f incident.Finding, ref *incident.ResourceRef) string {
	target := "workload"
	if ref != nil && ref.Name != "" {
		target = ref.Name
	}
	return fmt.Sprintf("describe %s", target)
}

func auditGuidanceSummary(code string) string {
	switch code {
	case "Audit.RunAsRoot":
		return "Set runAsNonRoot=true only with a non-root-capable image — enforcing it blindly can break container startup"
	case "Audit.HostNamespace":
		return "Remove hostNetwork/hostPID/hostIPC unless the workload genuinely needs the host namespace"
	case "Audit.LatestTag":
		return "Pin a specific tag or digest — kprompt never invents image tags"
	case "Audit.MissingRequests":
		return "Add resources.requests after profiling — CPU/memory values are workload-specific, not invented"
	case "Audit.MissingLimits":
		return "Add resources.limits after profiling — CPU/memory values are workload-specific, not invented"
	case "Audit.MissingImagePullPolicy":
		return "Set imagePullPolicy (e.g. IfNotPresent) once the image tag is pinned"
	case "Audit.WritableRootFS":
		return "Set securityContext.readOnlyRootFilesystem=true and mount an emptyDir for paths that need writes — kprompt never auto-enables it because a container that writes to its root filesystem would break"
	default:
		return "Review the finding and remediate in your workload manifest"
	}
}

func boolRef(v bool) *bool { return &v }
