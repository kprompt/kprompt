package planner

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/yaml"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/intent"
)

// Build creates an ExecutionPlan from a structured Intent.
func Build(in intent.Intent) (ExecutionPlan, error) {
	ns := in.Target.Namespace
	if ns == "" {
		ns = "default"
	}
	switch in.Kind {
	case intent.KindScale:
		return buildScale(in, ns)
	case intent.KindRollback:
		return buildRollback(in, ns)
	case intent.KindDeploy:
		return buildDeploy(in, ns)
	case intent.KindInstall:
		return buildInstall(in, ns)
	case intent.KindUpgrade:
		return buildUpgrade(in, ns)
	case intent.KindGet:
		return buildGet(in, ns)
	case intent.KindExplain:
		return buildExplain(in, ns)
	case intent.KindLogs:
		return buildLogs(in, ns)
	case intent.KindDescribe:
		return buildDescribe(in, ns)
	case intent.KindWorkflow:
		return buildWorkflow(in, ns)
	case intent.KindPerformance:
		return buildPerformance(in, ns)
	case intent.KindTrace:
		return buildTrace(in)
	case intent.KindDashboard:
		return buildDashboard(in)
	case intent.KindOptimize:
		return buildOptimize(in)
	case intent.KindDelete:
		return buildDelete(in, ns)
	case intent.KindDeny:
		return ExecutionPlan{Intent: in, Summary: "Denied intent", RequiresApproval: false}, nil
	default:
		return ExecutionPlan{}, fmt.Errorf("unsupported intent kind %q", in.Kind)
	}
}

func buildGet(in intent.Intent, ns string) (ExecutionPlan, error) {
	raw := first(in.Target.Kind, "Pod")
	ref, err := cluster.ParseResourceRef(raw)
	if err != nil {
		return ExecutionPlan{}, err
	}
	name := strings.TrimSpace(in.Target.Name)
	if ref.Kind == "Workflow" && name == "" {
		return ExecutionPlan{}, fmt.Errorf("get Workflow requires target.name")
	}

	req := cluster.ReadRequest{
		Resource:  ref,
		Namespace: ns,
		Name:      name,
	}
	if sel, ok := in.LabelSelector(); ok {
		req.LabelSelector = sel
	}
	if limit, ok := in.Limit(); ok {
		req.Limit = limit
	}
	if timeout, ok := in.Timeout(); ok {
		req.Timeout = timeout
	}
	req, err = cluster.NormalizeReadRequest(req)
	if err != nil {
		return ExecutionPlan{}, err
	}

	kind := req.Resource.Kind
	if kind == "" {
		kind = req.Resource.Resource
	}
	actionNS := req.Namespace
	summary := fmt.Sprintf("List %s", req.Resource.Display())
	if actionNS != "" {
		summary += " in " + actionNS
	} else if req.Resource.Scope == cluster.ScopeCluster {
		summary += " (cluster scope)"
	}
	if name != "" {
		summary = fmt.Sprintf("Get %s/%s", req.Resource.Display(), name)
		if actionNS != "" {
			summary += " in " + actionNS
		}
	}
	if req.LabelSelector != "" {
		summary += fmt.Sprintf(" selector=%s", req.LabelSelector)
	}
	if mem, ok := in.MinMemory(); ok {
		summary += fmt.Sprintf(" minMemory=%s", mem)
	}
	if in.Params == nil {
		in.Params = map[string]any{}
	}
	in.Params["resource"] = req.Resource.Qualified()
	if req.Resource.Group != "" {
		in.Params["group"] = req.Resource.Group
	}
	in.Params["limit"] = req.Limit
	in.Params["timeout"] = req.Timeout.String()
	in.Target.Kind = kind
	in.Target.Namespace = actionNS

	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op:      OpGet,
			Backend: "kubernetes",
			Object: ObjectRef{
				APIVersion: apiVersionForResource(req.Resource),
				Kind:       kind,
				Name:       name,
				Namespace:  actionNS,
			},
			Diff: summary,
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}

func apiVersionForResource(ref cluster.ResourceRef) string {
	switch {
	case ref.Group == "apps":
		return "apps/v1"
	case ref.Group == "argoproj.io":
		return "argoproj.io/v1alpha1"
	case ref.Group != "":
		if ref.Version != "" {
			return ref.Group + "/" + ref.Version
		}
		return ref.Group
	case ref.Kind == "Deployment":
		return "apps/v1"
	case ref.Kind == "Workflow":
		return "argoproj.io/v1alpha1"
	default:
		return "v1"
	}
}

func buildExplain(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("explain intent missing target.name")
	}
	kind := first(in.Target.Kind, "Deployment")
	kind = cluster.NormalizeKind(kind)
	if kind != "Pod" && kind != "Deployment" {
		kind = "Deployment"
	}
	summary := fmt.Sprintf("Explain %s/%s in %s (deployment chain + events + logs)", kind, name, ns)
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op: OpGet,
			Object: ObjectRef{
				Kind:      kind,
				Name:      name,
				Namespace: ns,
			},
			Diff: summary,
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}

func buildLogs(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("logs intent missing target.name")
	}
	kind := first(in.Target.Kind, "Deployment")
	kind = cluster.NormalizeKind(kind)
	if kind != "Pod" && kind != "Deployment" {
		kind = "Deployment"
	}
	tail := int64(100)
	if t, ok := in.TailLines(); ok {
		if t < 1 {
			return ExecutionPlan{}, fmt.Errorf("params.tail must be >= 1")
		}
		if t > 5000 {
			t = 5000
		}
		tail = t
	}
	summary := fmt.Sprintf("Logs for %s/%s in %s (last %d lines)", kind, name, ns, tail)
	if c, ok := in.Container(); ok {
		summary += fmt.Sprintf(" container=%s", c)
	}
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op: OpGet,
			Object: ObjectRef{
				Kind:      kind,
				Name:      name,
				Namespace: ns,
			},
			Diff: summary,
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}

func buildDescribe(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("describe intent missing target.name")
	}
	kind := first(in.Target.Kind, "Deployment")
	kind = cluster.NormalizeKind(kind)
	if kind != "Pod" && kind != "Deployment" {
		kind = "Deployment"
	}
	summary := fmt.Sprintf("Describe %s/%s in %s (compact)", kind, name, ns)
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op: OpGet,
			Object: ObjectRef{
				Kind:      kind,
				Name:      name,
				Namespace: ns,
			},
			Diff: summary,
		}},
		Summary:          summary,
		RequiresApproval: false,
	}, nil
}

func buildDelete(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("delete intent missing target.name (named deletes only)")
	}
	if isUnscopedName(name) {
		return ExecutionPlan{}, fmt.Errorf("delete refuses unscoped name %q", name)
	}
	kind := strings.TrimSpace(in.Target.Kind)
	if kind == "" {
		kind = "Deployment"
	}
	kind = cluster.NormalizeKind(kind)
	switch kind {
	case "Pod", "Deployment", "Service":
	default:
		return ExecutionPlan{}, fmt.Errorf("delete kind %q not supported (Pod, Deployment, Service only; namespace wipe denied)", in.Target.Kind)
	}
	summary := fmt.Sprintf("Delete %s/%s in %s", kind, name, ns)
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op: OpDelete,
			Object: ObjectRef{
				APIVersion: apiVersionForKind(kind),
				Kind:       kind,
				Name:       name,
				Namespace:  ns,
			},
			Diff: summary,
		}},
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}

func isUnscopedName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "*", "all", "everything", "--all", "any":
		return true
	default:
		return false
	}
}

func apiVersionForKind(kind string) string {
	switch kind {
	case "Deployment":
		return "apps/v1"
	case "Workflow":
		return "argoproj.io/v1alpha1"
	default:
		return "v1"
	}
}

func buildScale(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := in.Target.Name
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("scale intent missing target.name")
	}
	replicas, ok := in.Replicas()
	if !ok || replicas < 0 {
		return ExecutionPlan{}, fmt.Errorf("scale intent missing valid params.replicas")
	}
	kind := in.Target.Kind
	if kind == "" {
		kind = "Deployment"
	}
	rep := replicas
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op: OpScale,
			Object: ObjectRef{
				APIVersion: "apps/v1",
				Kind:       kind,
				Name:       name,
				Namespace:  ns,
			},
			Replicas: &rep,
			Diff:     fmt.Sprintf("scale %s/%s to %d replicas", kind, name, replicas),
		}},
		Summary:          fmt.Sprintf("Scale %s/%s in %s to %d replicas", kind, name, ns, replicas),
		RequiresApproval: true,
	}, nil
}

func buildRollback(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("rollback intent missing target.name")
	}
	kind := first(in.Target.Kind, "Deployment")
	kind = cluster.NormalizeKind(kind)
	if kind != "Deployment" {
		return ExecutionPlan{}, fmt.Errorf("rollback supports Deployment only (got %q)", kind)
	}
	var rev *int64
	summary := fmt.Sprintf("Rollback Deployment/%s in %s to previous revision", name, ns)
	diff := fmt.Sprintf("rollout undo Deployment/%s", name)
	if r, ok := in.Revision(); ok {
		if r < 1 {
			return ExecutionPlan{}, fmt.Errorf("params.revision must be >= 1 when set")
		}
		rev = &r
		summary = fmt.Sprintf("Rollback Deployment/%s in %s to revision %d", name, ns, r)
		diff = fmt.Sprintf("rollout undo Deployment/%s --to-revision=%d", name, r)
	}
	return ExecutionPlan{
		Intent: in,
		Actions: []Action{{
			Op: OpRollback,
			Object: ObjectRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       name,
				Namespace:  ns,
			},
			Revision: rev,
			Diff:     diff,
		}},
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}

func buildDeploy(in intent.Intent, ns string) (ExecutionPlan, error) {
	name := strings.TrimSpace(in.Target.Name)
	if name == "" {
		return ExecutionPlan{}, fmt.Errorf("deploy intent missing target.name")
	}
	image, err := resolveImage(name, in)
	if err != nil {
		return ExecutionPlan{}, err
	}
	replicas := int32(1)
	if r, ok := in.Replicas(); ok && r > 0 {
		replicas = r
	}
	port, hasPort := in.Port()
	if !hasPort {
		if p, ok := defaultPort(name, image); ok {
			port = p
			hasPort = true
		}
	}
	wantSvc := in.WantService() || hasPort

	labels := map[string]string{"app": name, "app.kubernetes.io/managed-by": "kprompt"}
	dep := &appsv1.Deployment{
		TypeMeta: metav1.TypeMeta{APIVersion: "apps/v1", Kind: "Deployment"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: ns,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  sanitizeContainerName(name),
						Image: image,
					}},
				},
			},
		},
	}
	if hasPort && port > 0 {
		dep.Spec.Template.Spec.Containers[0].Ports = []corev1.ContainerPort{{
			ContainerPort: port,
			Name:          "http",
		}}
	}

	depYAML, err := yaml.Marshal(dep)
	if err != nil {
		return ExecutionPlan{}, err
	}

	actions := []Action{{
		Op: OpCreate,
		Object: ObjectRef{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       name,
			Namespace:  ns,
		},
		Manifest: string(depYAML),
		Replicas: &replicas,
		Diff:     fmt.Sprintf("create Deployment/%s image=%s replicas=%d", name, image, replicas),
	}}

	summary := fmt.Sprintf("Deploy %s (%s) in %s with %d replica(s)", name, image, ns, replicas)

	if wantSvc && hasPort && port > 0 {
		svc := &corev1.Service{
			TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "Service"},
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: ns,
				Labels:    labels,
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": name},
				Ports: []corev1.ServicePort{{
					Name:       "http",
					Port:       port,
					TargetPort: intstr.FromInt32(port),
				}},
				Type: corev1.ServiceTypeClusterIP,
			},
		}
		svcYAML, err := yaml.Marshal(svc)
		if err != nil {
			return ExecutionPlan{}, err
		}
		actions = append(actions, Action{
			Op: OpCreate,
			Object: ObjectRef{
				APIVersion: "v1",
				Kind:       "Service",
				Name:       name,
				Namespace:  ns,
			},
			Manifest: string(svcYAML),
			Diff:     fmt.Sprintf("create Service/%s port=%d", name, port),
		})
		summary += fmt.Sprintf(" + Service :%d", port)
	}

	return ExecutionPlan{
		Intent:           in,
		Actions:          actions,
		Summary:          summary,
		RequiresApproval: true,
	}, nil
}

func resolveImage(name string, in intent.Intent) (string, error) {
	if img, ok := in.Image(); ok {
		return img, nil
	}
	switch strings.ToLower(name) {
	case "redis":
		return "redis:7-alpine", nil
	case "nginx":
		return "nginx:1.27-alpine", nil
	default:
		// Allow image-like names (repo/name:tag).
		if strings.Contains(name, "/") || strings.Contains(name, ":") {
			return name, nil
		}
		return "", fmt.Errorf("deploy intent missing params.image for %q (known shortcuts: redis, nginx)", name)
	}
}

func defaultPort(name, image string) (int32, bool) {
	key := strings.ToLower(name + " " + image)
	switch {
	case strings.Contains(key, "redis"):
		return 6379, true
	case strings.Contains(key, "nginx"):
		return 80, true
	default:
		return 0, false
	}
}

func sanitizeContainerName(name string) string {
	name = strings.ToLower(name)
	name = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			return r
		}
		return '-'
	}, name)
	name = strings.Trim(name, "-")
	if name == "" {
		return "app"
	}
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
