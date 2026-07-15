package suggest

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/cluster"
)

func TestSuggestOOMRaisesMemory(t *testing.T) {
	limit := resource.MustParse("128Mi")
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "demo"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "app:1",
						Resources: corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: limit},
						},
					}},
				},
			},
		},
	})
	suggestions, err := FromExplain(context.Background(), client, cluster.ExplainReport{
		Target: "api", Namespace: "demo", Kind: "Deployment",
		Findings: []cluster.Finding{{Code: "OOMKilled", Container: "app", Severity: "error"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 1 || suggestions[0].Plan == nil {
		t.Fatalf("%+v", suggestions)
	}
	if !strings.Contains(suggestions[0].Plan.Summary, "128Mi") {
		t.Fatalf("summary=%s", suggestions[0].Plan.Summary)
	}
}

func TestSuggestCrashLoopPromptOnly(t *testing.T) {
	suggestions, err := FromExplain(context.Background(), fake.NewSimpleClientset(), cluster.ExplainReport{
		Target: "crashy", Namespace: "demo", Kind: "Deployment",
		Findings: []cluster.Finding{{Code: "CrashLoopBackOff", Container: "crash"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(suggestions) != 1 || suggestions[0].Plan != nil {
		t.Fatalf("%+v", suggestions)
	}
	if suggestions[0].Prompt != "logs crashy" {
		t.Fatalf("prompt=%q", suggestions[0].Prompt)
	}
}

func TestBumpMemoryDefault(t *testing.T) {
	old, neu := bumpMemory(nil)
	if !old.IsZero() || neu.String() != "256Mi" {
		t.Fatalf("old=%s new=%s", old.String(), neu.String())
	}
}
