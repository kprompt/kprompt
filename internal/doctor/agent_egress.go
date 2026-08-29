package doctor

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/cluster"
)

const kpromptWorkloadLabelSelector = "app.kubernetes.io/name in (kprompt-agent,kprompt-coordinator,kprompt-operator)"

// AgentEgressSummary is the cluster posture used by the NetworkPolicy doctor check.
type AgentEgressSummary struct {
	AgentPods           int
	HostNetworkPods     int
	PodsWithoutEgressNP int
	SampleNamespaces    []string
}

// ProbeAgentEgress inspects kprompt workload pods and selecting egress NetworkPolicies.
// nil means use the default kube implementation.
type ProbeAgentEgress func(ctx context.Context, kubeCtx string) (AgentEgressSummary, error)

func defaultProbeAgentEgress(ctx context.Context, kubeCtx string) (AgentEgressSummary, error) {
	cl, err := cluster.Connect(kubeCtx)
	if err != nil {
		return AgentEgressSummary{}, err
	}
	return summarizeAgentEgress(ctx, cl.Clientset)
}

func summarizeAgentEgress(ctx context.Context, cs kubernetes.Interface) (AgentEgressSummary, error) {
	pods, err := cs.CoreV1().Pods("").List(ctx, metav1.ListOptions{
		LabelSelector: kpromptWorkloadLabelSelector,
	})
	if err != nil {
		return AgentEgressSummary{}, err
	}
	if len(pods.Items) == 0 {
		return AgentEgressSummary{}, nil
	}

	byNS := map[string][]corev1.Pod{}
	var hostNet int
	nsOrder := []string{}
	seenNS := map[string]bool{}
	for _, p := range pods.Items {
		if p.Spec.HostNetwork {
			hostNet++
		}
		ns := p.Namespace
		if !seenNS[ns] {
			seenNS[ns] = true
			nsOrder = append(nsOrder, ns)
		}
		byNS[ns] = append(byNS[ns], p)
	}

	var uncovered int
	for ns, nsPods := range byNS {
		nps, err := cs.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return AgentEgressSummary{}, err
		}
		for _, p := range nsPods {
			if !podHasEgressNetworkPolicy(p, nps.Items) {
				uncovered++
			}
		}
	}

	sample := nsOrder
	if len(sample) > 3 {
		sample = sample[:3]
	}
	return AgentEgressSummary{
		AgentPods:           len(pods.Items),
		HostNetworkPods:     hostNet,
		PodsWithoutEgressNP: uncovered,
		SampleNamespaces:    sample,
	}, nil
}

func podHasEgressNetworkPolicy(pod corev1.Pod, policies []networkingv1.NetworkPolicy) bool {
	for _, np := range policies {
		if !networkPolicyHasEgress(np) {
			continue
		}
		if networkPolicySelectsPod(np, pod) {
			return true
		}
	}
	return false
}

func networkPolicyHasEgress(np networkingv1.NetworkPolicy) bool {
	if len(np.Spec.PolicyTypes) == 0 {
		// Legacy: egress rules imply egress policy.
		return len(np.Spec.Egress) > 0
	}
	for _, t := range np.Spec.PolicyTypes {
		if t == networkingv1.PolicyTypeEgress {
			return true
		}
	}
	return false
}

func networkPolicySelectsPod(np networkingv1.NetworkPolicy, pod corev1.Pod) bool {
	sel, err := metav1.LabelSelectorAsSelector(&np.Spec.PodSelector)
	if err != nil {
		return false
	}
	// Empty selector matches all pods in the namespace.
	if sel.Empty() && len(np.Spec.PodSelector.MatchLabels) == 0 && len(np.Spec.PodSelector.MatchExpressions) == 0 {
		return true
	}
	return sel.Matches(labels.Set(pod.Labels))
}

func checkAgentEgress(sum AgentEgressSummary, probeErr error) Check {
	c := Check{
		ID:       "agent-egress",
		Name:     "Workload NetworkPolicy",
		Required: false,
	}
	if probeErr != nil {
		c.Status = Skip
		c.Detail = "could not inspect NetworkPolicy posture"
		c.Hint = "Ensure the current context can list pods/networkpolicies, or skip until charts are installed"
		return c
	}
	if sum.AgentPods == 0 {
		c.Status = Skip
		c.Detail = "no kprompt agent/coordinator/operator pods found"
		c.Hint = "Install charts/kprompt-agent (or coordinator/operator); see docs/security/operator-endpoint-hardening.md"
		return c
	}

	var parts []string
	parts = append(parts, fmt.Sprintf("%d kprompt pod(s)", sum.AgentPods))
	if len(sum.SampleNamespaces) > 0 {
		parts = append(parts, "ns="+strings.Join(sum.SampleNamespaces, ","))
	}

	warns := []string{}
	if sum.HostNetworkPods > 0 {
		warns = append(warns, fmt.Sprintf("%d with hostNetwork=true (bypasses NetworkPolicy)", sum.HostNetworkPods))
	}
	if sum.PodsWithoutEgressNP > 0 {
		warns = append(warns, fmt.Sprintf("%d without selecting egress NetworkPolicy", sum.PodsWithoutEgressNP))
	}

	if len(warns) == 0 {
		c.Status = Pass
		c.Detail = strings.Join(parts, " · ") + " · egress NetworkPolicy present"
		return c
	}

	c.Status = Warn
	c.Detail = strings.Join(parts, " · ") + " · " + strings.Join(warns, "; ")
	c.Hint = "Enable chart networkPolicy.enabled with CIDRs — docs/security/operator-endpoint-hardening.md (SEC-007). Do not use hostNetwork."
	return c
}
