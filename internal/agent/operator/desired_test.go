package operator

import (
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	agentv1 "github.com/kprompt/kprompt/api/v1"
)

func TestBuildDesiredObserve(t *testing.T) {
	cr := &agentv1.KpromptAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "demo", Namespace: "payments", UID: "uid-1"},
		Spec: agentv1.KpromptAgentSpec{
			Mode:      agentv1.ModeObserve,
			LLM:       agentv1.LLMSpec{Provider: "openai", Heuristic: true},
			Notify:    agentv1.NotifySpec{Slack: true, Discord: true},
			SecretRef: &agentv1.SecretRef{Name: "kprompt-agent"},
			Watches:   []string{"pods", "events", "deployments"},
		},
	}
	d, err := BuildDesired(cr, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if d.Deployment.Namespace != "payments" || d.SA.Name != "kprompt-agent-demo" {
		t.Fatalf("ns/name: %+v %+v", d.Deployment.Namespace, d.SA.Name)
	}
	args := d.Deployment.Spec.Template.Spec.Containers[0].Args
	joined := ""
	for _, a := range args {
		joined += a + " "
	}
	for _, want := range []string{"--in-cluster", "--heuristic", "--slack", "--watch", "--agent-cr"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("args missing %s: %v", want, args)
		}
	}
	if len(d.Role.Rules) < 4 {
		t.Fatalf("expected observe rules, got %d", len(d.Role.Rules))
	}
}

func TestRejectAutopilot(t *testing.T) {
	cr := &agentv1.KpromptAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"},
		Spec:       agentv1.KpromptAgentSpec{Mode: "Autopilot"},
	}
	if _, err := BuildDesired(cr, Options{}); err == nil {
		t.Fatal("expected Autopilot reject")
	}
}

func TestRejectCrossNamespace(t *testing.T) {
	cr := &agentv1.KpromptAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ops"},
		Spec:       agentv1.KpromptAgentSpec{Mode: agentv1.ModeObserve, Namespace: "payments"},
	}
	if _, err := BuildDesired(cr, Options{}); err == nil {
		t.Fatal("expected cross-ns reject")
	}
}

func TestDefaultModeEmpty(t *testing.T) {
	cr := &agentv1.KpromptAgent{
		ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: "ns"},
		Spec:       agentv1.KpromptAgentSpec{},
	}
	if err := ValidateMode(cr); err != nil {
		t.Fatal(err)
	}
	d, err := BuildDesired(cr, Options{DefaultImage: "example.com/kprompt:dev"})
	if err != nil {
		t.Fatal(err)
	}
	if d.Deployment.Spec.Template.Spec.Containers[0].Image != "example.com/kprompt:dev" {
		t.Fatalf("image=%s", d.Deployment.Spec.Template.Spec.Containers[0].Image)
	}
}
