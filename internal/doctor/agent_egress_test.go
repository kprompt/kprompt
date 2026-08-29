package doctor

import (
	"context"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kprompt/kprompt/internal/team"
	"github.com/kprompt/kprompt/internal/tools"
)

func TestCheckAgentEgressSkipNoPods(t *testing.T) {
	c := checkAgentEgress(AgentEgressSummary{}, nil)
	if c.Status != Skip || c.ID != "agent-egress" {
		t.Fatalf("%+v", c)
	}
}

func TestCheckAgentEgressWarnWithoutNP(t *testing.T) {
	c := checkAgentEgress(AgentEgressSummary{
		AgentPods:           2,
		PodsWithoutEgressNP: 2,
		SampleNamespaces:    []string{"payments"},
	}, nil)
	if c.Status != Warn {
		t.Fatalf("%+v", c)
	}
	if !strings.Contains(c.Detail, "without selecting egress") {
		t.Fatalf("detail=%q", c.Detail)
	}
	if !strings.Contains(c.Hint, "operator-endpoint-hardening") {
		t.Fatalf("hint=%q", c.Hint)
	}
}

func TestCheckAgentEgressWarnHostNetwork(t *testing.T) {
	c := checkAgentEgress(AgentEgressSummary{
		AgentPods:           1,
		HostNetworkPods:     1,
		PodsWithoutEgressNP: 0,
	}, nil)
	if c.Status != Warn || !strings.Contains(c.Detail, "hostNetwork") {
		t.Fatalf("%+v", c)
	}
}

func TestCheckAgentEgressPass(t *testing.T) {
	c := checkAgentEgress(AgentEgressSummary{AgentPods: 1, PodsWithoutEgressNP: 0}, nil)
	if c.Status != Pass {
		t.Fatalf("%+v", c)
	}
}

func TestNetworkPolicySelectsPod(t *testing.T) {
	pod := corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "agent",
			Namespace: "payments",
			Labels:    map[string]string{"app.kubernetes.io/name": "kprompt-agent"},
		},
	}
	np := networkingv1.NetworkPolicy{
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": "kprompt-agent"},
			},
			PolicyTypes: []networkingv1.PolicyType{networkingv1.PolicyTypeEgress},
			Egress:      []networkingv1.NetworkPolicyEgressRule{{}},
		},
	}
	if !podHasEgressNetworkPolicy(pod, []networkingv1.NetworkPolicy{np}) {
		t.Fatal("expected match")
	}
	ingressOnly := np
	ingressOnly.Spec.PolicyTypes = []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}
	ingressOnly.Spec.Egress = nil
	if podHasEgressNetworkPolicy(pod, []networkingv1.NetworkPolicy{ingressOnly}) {
		t.Fatal("ingress-only must not count")
	}
}

func TestRunIncludesAgentEgressWhenKubeOK(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", dir+"/.kprompt")
	t.Setenv("KPROMPT_OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	rep, err := Run(context.Background(), Options{
		Detect: func(context.Context, tools.DetectOptions) (*tools.Registry, error) {
			return tools.NewRegistry([]tools.Result{
				{ID: tools.IDKubernetes, Name: "Kubernetes", Status: tools.StatusAvailable, Detail: "ok"},
			}), nil
		},
		ProbeAgentEgress: func(context.Context, string) (AgentEgressSummary, error) {
			return AgentEgressSummary{AgentPods: 1, PodsWithoutEgressNP: 1, SampleNamespaces: []string{"ns"}}, nil
		},
		Me: func(context.Context, string, string) (team.MeResponse, error) {
			t.Fatal("me")
			return team.MeResponse{}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	c := find(rep, "agent-egress")
	if c.Status != Warn {
		t.Fatalf("%+v", c)
	}
	// LLM fails without provider, but advisory must not flip required fail alone.
	if find(rep, "agent-egress").Required {
		t.Fatal("agent-egress must be optional")
	}
}
