//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestExplainCrashLoopOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)
	ensureCrashDeployment(t, ctx, client, "crashy")

	// Wait briefly for CrashLoopBackOff / failed state.
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		pods, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{LabelSelector: "app=crashy"})
		if err == nil && len(pods.Items) > 0 {
			for _, p := range pods.Items {
				for _, cs := range p.Status.ContainerStatuses {
					if cs.RestartCount > 0 || (cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff") {
						goto ready
					}
					if cs.LastTerminationState.Terminated != nil && cs.LastTerminationState.Terminated.ExitCode != 0 {
						goto ready
					}
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
ready:

	var out bytes.Buffer
	cfg := config.Resolved{
		Provider:  "stub",
		Namespace: ns,
		Prompt:    "explain why crashy is crashing",
	}
	err := pipeline.RunWith(ctx, cfg, &out, pipeline.Deps{
		Provider: llm.ExplainStub("crashy", ns, "Deployment"),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("pipeline: %v\noutput:\n%s", err, out.String())
	}
	text := out.String()
	t.Log(text)
	if !strings.Contains(text, "Summary:") {
		t.Fatal("expected summary")
	}
	// Soft assert: heuristic or restart-related signal.
	if !strings.Contains(text, "CrashLoopBackOff") &&
		!strings.Contains(text, "exit code") &&
		!strings.Contains(text, "Restarts") &&
		!strings.Contains(text, "Findings:") {
		t.Fatalf("expected crash-related findings, got:\n%s", text)
	}
}

func ensureCrashDeployment(t *testing.T, ctx context.Context, client kubernetes.Interface, name string) {
	t.Helper()
	_ = client.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{})
	replicas := int32(1)
	labels := map[string]string{"app": name}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyAlways,
					Containers: []corev1.Container{{
						Name:    "crash",
						Image:   "busybox:1.36",
						Command: []string{"sh", "-c", "exit 1"},
					}},
				},
			},
		},
	}
	if _, err := client.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}
