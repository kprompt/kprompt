// Package operator reconciles KpromptAgent CRs into Observe agent workloads (AG-014).
//
// It creates ServiceAccount + Role + RoleBinding + Deployment in the watch
// namespace. Autopilot / mutate modes are refused (ADR-0013).
package operator

import (
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"

	agentv1 "github.com/kprompt/kprompt/api/v1"
)

const (
	LabelManagedBy = "app.kubernetes.io/managed-by"
	LabelName      = "app.kubernetes.io/name"
	LabelInstance  = "app.kubernetes.io/instance"
	LabelComponent = "app.kubernetes.io/component"
	ManagedByValue = "kprompt-operator"
	AppName        = "kprompt-agent"
	ComponentAgent = "observe-agent"

	DefaultImageRepo = "ghcr.io/kprompt/kprompt"
	DefaultImageTag  = "latest"
)

// Options configures desired-state rendering.
type Options struct {
	DefaultImage string // repo:tag; empty → DefaultImageRepo:DefaultImageTag
}

// Desired is the set of owned objects for one KpromptAgent.
type Desired struct {
	WatchNamespace string
	SA             *corev1.ServiceAccount
	Role           *rbacv1.Role
	RoleBinding    *rbacv1.RoleBinding
	Deployment     *appsv1.Deployment
}

// WatchNamespace resolves where the agent runs (spec.namespace or CR namespace).
func WatchNamespace(cr *agentv1.KpromptAgent) string {
	if cr == nil {
		return ""
	}
	if ns := strings.TrimSpace(cr.Spec.Namespace); ns != "" {
		return ns
	}
	return cr.Namespace
}

// ValidateMode ensures Observe-only. Empty mode defaults to Observe.
func ValidateMode(cr *agentv1.KpromptAgent) error {
	mode := agentv1.DefaultMode(cr.Spec.Mode)
	if mode != agentv1.ModeObserve {
		return fmt.Errorf("mode %q is not supported; only %q (Autopilot requires a future ADR)", mode, agentv1.ModeObserve)
	}
	return nil
}

// BuildDesired returns SA/Role/RB/Deployment owned by the CR.
func BuildDesired(cr *agentv1.KpromptAgent, opts Options) (*Desired, error) {
	if cr == nil {
		return nil, fmt.Errorf("operator: KpromptAgent is nil")
	}
	if err := ValidateMode(cr); err != nil {
		return nil, err
	}
	ns := WatchNamespace(cr)
	if ns == "" {
		return nil, fmt.Errorf("operator: watch namespace is empty")
	}
	if ns != cr.Namespace {
		return nil, fmt.Errorf("operator: spec.namespace %q must equal CR namespace %q in V1 (cross-namespace agent is out of scope)", ns, cr.Namespace)
	}
	name := ResourceName(cr)
	owner := ownerRef(cr)
	labels := map[string]string{
		LabelManagedBy: ManagedByValue,
		LabelName:      AppName,
		LabelInstance:  cr.Name,
		LabelComponent: ComponentAgent,
	}
	image := resolveImage(cr, opts)

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Rules: observeRoleRules(cr),
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     name,
		},
		Subjects: []rbacv1.Subject{{
			Kind:      "ServiceAccount",
			Name:      name,
			Namespace: ns,
		}},
	}

	replicas := int32(1)
	args := agentArgs(cr, ns)
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:            name,
			Namespace:       ns,
			Labels:          labels,
			OwnerReferences: []metav1.OwnerReference{owner},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					ServiceAccountName: name,
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						SeccompProfile: &corev1.SeccompProfile{
							Type: corev1.SeccompProfileTypeRuntimeDefault,
						},
					},
					Containers: []corev1.Container{{
						Name:            "agent",
						Image:           image,
						ImagePullPolicy: corev1.PullIfNotPresent,
						Args:            args,
						Env:             agentEnv(cr, ns),
						EnvFrom:         agentEnvFrom(cr),
						SecurityContext: &corev1.SecurityContext{
							AllowPrivilegeEscalation: boolPtr(false),
							ReadOnlyRootFilesystem:   boolPtr(true),
							Capabilities:             &corev1.Capabilities{Drop: []corev1.Capability{"ALL"}},
						},
						Resources: corev1.ResourceRequirements{},
					}},
				},
			},
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 0},
					MaxSurge:       &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
				},
			},
		},
	}

	return &Desired{
		WatchNamespace: ns,
		SA:             sa,
		Role:           role,
		RoleBinding:    rb,
		Deployment:     dep,
	}, nil
}

// ResourceName is the shared name for owned objects.
func ResourceName(cr *agentv1.KpromptAgent) string {
	return "kprompt-agent-" + cr.Name
}

func ownerRef(cr *agentv1.KpromptAgent) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         schema.GroupVersion{Group: agentv1.Group, Version: agentv1.Version}.String(),
		Kind:               agentv1.Kind,
		Name:               cr.Name,
		UID:                cr.UID,
		Controller:         boolPtr(true),
		BlockOwnerDeletion: boolPtr(true),
	}
}

func resolveImage(cr *agentv1.KpromptAgent, opts Options) string {
	if cr.Spec.Image != nil {
		repo := strings.TrimSpace(cr.Spec.Image.Repository)
		tag := strings.TrimSpace(cr.Spec.Image.Tag)
		if repo != "" {
			if tag == "" {
				tag = DefaultImageTag
			}
			return repo + ":" + tag
		}
	}
	if img := strings.TrimSpace(opts.DefaultImage); img != "" {
		return img
	}
	return DefaultImageRepo + ":" + DefaultImageTag
}

func agentArgs(cr *agentv1.KpromptAgent, watchNS string) []string {
	args := []string{"agent", "run", "--namespace", watchNS, "--in-cluster", "--analyze", "--agent-cr", cr.Name, "--agent-cr-namespace", cr.Namespace}
	if boolOr(cr.Spec.FetchLogs, true) {
		args = append(args, "--fetch-logs")
	}
	if boolOr(cr.Spec.Health, true) {
		args = append(args, "--health")
	}
	if cr.Spec.LLM.Heuristic {
		args = append(args, "--heuristic")
	}
	if cr.Spec.Notify.Discord {
		args = append(args, "--discord")
	}
	if cr.Spec.Notify.Slack {
		args = append(args, "--slack")
	}
	if cr.Spec.Notify.Webhook {
		args = append(args, "--webhook")
	}
	if p := strings.TrimSpace(cr.Spec.LLM.Provider); p != "" {
		args = append(args, "--provider", p)
	}
	if m := strings.TrimSpace(cr.Spec.LLM.Model); m != "" {
		args = append(args, "--model", m)
	}
	if s := strings.TrimSpace(cr.Spec.Severity.Minimum); s != "" {
		args = append(args, "--min-severity", s)
	}
	if cr.Spec.Severity.MinConfidence > 0 {
		args = append(args, "--min-confidence", fmt.Sprintf("%g", cr.Spec.Severity.MinConfidence))
	}
	if len(cr.Spec.Watches) > 0 {
		args = append(args, "--watch", strings.Join(cr.Spec.Watches, ","))
	}
	return args
}

func agentEnv(cr *agentv1.KpromptAgent, watchNS string) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "KPROMPT_WATCH_NAMESPACE", Value: watchNS},
		{Name: "POD_NAMESPACE", ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: "metadata.namespace"},
		}},
		{Name: "KPROMPT_AGENT_CR", Value: cr.Name},
		{Name: "KPROMPT_AGENT_CR_NAMESPACE", Value: cr.Namespace},
	}
}

func agentEnvFrom(cr *agentv1.KpromptAgent) []corev1.EnvFromSource {
	if cr.Spec.SecretRef == nil || strings.TrimSpace(cr.Spec.SecretRef.Name) == "" {
		return nil
	}
	return []corev1.EnvFromSource{{
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: cr.Spec.SecretRef.Name},
			Optional:             boolPtr(true),
		},
	}}
}

func observeRoleRules(cr *agentv1.KpromptAgent) []rbacv1.PolicyRule {
	rules := []rbacv1.PolicyRule{
		{
			APIGroups: []string{""},
			Resources: []string{"pods", "pods/log", "events", "configmaps", "services", "resourcequotas", "limitranges"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"apps"},
			Resources: []string{"deployments", "replicasets", "statefulsets"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"batch"},
			Resources: []string{"jobs", "cronjobs"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{""},
			Resources: []string{"persistentvolumeclaims"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"networking.k8s.io"},
			Resources: []string{"ingresses"},
			Verbs:     []string{"get", "list", "watch"},
		},
		{
			APIGroups: []string{"autoscaling"},
			Resources: []string{"horizontalpodautoscalers"},
			Verbs:     []string{"get", "list", "watch"},
		},
		// Status sync onto this CR (AG-013).
		{
			APIGroups: []string{agentv1.Group},
			Resources: []string{"kpromptagents", "kpromptagents/status"},
			Verbs:     []string{"get", "list", "watch", "patch", "update"},
		},
	}
	for _, w := range cr.Spec.Watches {
		if strings.EqualFold(strings.TrimSpace(w), "secrets") || strings.EqualFold(strings.TrimSpace(w), "secret") {
			rules = append(rules, rbacv1.PolicyRule{
				APIGroups: []string{""},
				Resources: []string{"secrets"},
				Verbs:     []string{"get", "list", "watch"},
			})
			break
		}
	}
	return rules
}

func boolOr(p *bool, def bool) bool {
	if p == nil {
		return def
	}
	return *p
}

func boolPtr(b bool) *bool { return &b }
