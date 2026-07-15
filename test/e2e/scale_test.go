//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

const (
	clusterName = "kprompt-e2e"
	ns          = "kprompt-e2e"
	deployName  = "demo"
)

func TestScaleDeploymentOnKind(t *testing.T) {
	if os.Getenv("KPROMPT_E2E") == "" && os.Getenv("CI") == "" {
		// Allow explicit local runs with -tags=e2e without env; still skip if no kind.
	}
	if _, err := exec.LookPath("kind"); err != nil {
		t.Skip("kind not installed")
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("kubectl not installed")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)
	ensureDeployment(t, ctx, client, 1)

	var out bytes.Buffer
	cfg := config.Resolved{
		Provider:  "stub",
		Namespace: ns,
		Approve:   true,
		Prompt:    "scale demo to 3",
	}
	err := pipeline.RunWith(ctx, cfg, &out, pipeline.Deps{
		Provider: llm.ScaleStub(deployName, ns, 3),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("pipeline: %v\noutput:\n%s", err, out.String())
	}
	t.Log(out.String())

	dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 3 {
		t.Fatalf("expected 3 replicas, got %v", dep.Spec.Replicas)
	}
}

func ensureKindCluster(t *testing.T, ctx context.Context) {
	t.Helper()
	out, err := exec.CommandContext(ctx, "kind", "get", "clusters").CombinedOutput()
	if err != nil {
		t.Fatalf("kind get clusters: %v (%s)", err, out)
	}
	if !bytes.Contains(out, []byte(clusterName)) {
		t.Logf("creating kind cluster %s …", clusterName)
		cmd := exec.CommandContext(ctx, "kind", "create", "cluster", "--name", clusterName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("kind create cluster: %v", err)
		}
	}
}

func exportKubeconfig(t *testing.T, ctx context.Context) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	cmd := exec.CommandContext(ctx, "kind", "export", "kubeconfig", "--name", clusterName, "--kubeconfig", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export kubeconfig: %v (%s)", err, out)
	}
	return path
}

func clientFromKubeconfig(t *testing.T, path string) *kubernetes.Clientset {
	t.Helper()
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatal(err)
	}
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func ensureNamespace(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	_, err := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return
	}
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func ensureDeployment(t *testing.T, ctx context.Context, client kubernetes.Interface, replicas int32) {
	t.Helper()
	_, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err == nil {
		// Reset to known replica count for the test.
		retryScale(t, ctx, client, replicas)
		return
	}
	labels := map[string]string{"app": deployName}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:1.27-alpine",
					}},
				},
			},
		},
	}
	if _, err := client.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func retryScale(t *testing.T, ctx context.Context, client kubernetes.Interface, replicas int32) {
	t.Helper()
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dep.Spec.Replicas = &replicas
	if _, err := client.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}
