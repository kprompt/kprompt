//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"fmt"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestRollbackDeploymentOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)
	ensureDeployment(t, ctx, client, 1)
	bumpDeploymentImage(t, ctx, client, "nginx:1.27-alpine")
	waitDeploymentImage(t, ctx, client, "nginx:1.27-alpine")
	waitDeploymentRevisionAtLeast(t, ctx, client, 1)
	bumpDeploymentImage(t, ctx, client, "nginx:1.26-alpine")
	waitDeploymentImage(t, ctx, client, "nginx:1.26-alpine")
	waitAtLeastTwoRevisions(t, ctx, client)
	waitDeploymentRevisionAtLeast(t, ctx, client, 2)

	var out bytes.Buffer
	cfg := config.Resolved{
		Provider:  "stub",
		Namespace: ns,
		Approve:   true,
		Prompt:    "rollback demo",
	}
	err := pipeline.RunWith(ctx, cfg, &out, pipeline.Deps{
		Provider: llm.RollbackStub(deployName, ns, 0),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("pipeline: %v\noutput:\n%s", err, out.String())
	}
	t.Log(out.String())

	waitDeploymentImage(t, ctx, client, "nginx:1.27-alpine")
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Template.Spec.Containers[0].Image != "nginx:1.27-alpine" {
		t.Fatalf("expected rolled-back image nginx:1.27-alpine, got %s", dep.Spec.Template.Spec.Containers[0].Image)
	}
}

func bumpDeploymentImage(t *testing.T, ctx context.Context, client kubernetes.Interface, image string) {
	t.Helper()
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dep.Spec.Template.Spec.Containers[0].Image = image
	if _, err := client.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func waitDeploymentImage(t *testing.T, ctx context.Context, client kubernetes.Interface, image string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
		if err == nil && len(dep.Spec.Template.Spec.Containers) > 0 &&
			dep.Spec.Template.Spec.Containers[0].Image == image {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for Deployment/%s image %s", deployName, image)
}

func waitAtLeastTwoRevisions(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		list, err := client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
		if err == nil {
			n := 0
			for _, rs := range list.Items {
				for _, ow := range rs.OwnerReferences {
					if ow.Kind == "Deployment" && ow.Name == deployName && ow.Controller != nil && *ow.Controller {
						n++
					}
				}
			}
			if n >= 2 {
				return
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Fatal("timed out waiting for at least two ReplicaSets (revisions)")
}

func waitDeploymentRevisionAtLeast(t *testing.T, ctx context.Context, client kubernetes.Interface, min int64) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	for time.Now().Before(deadline) {
		dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
		if err == nil && dep.Annotations != nil {
			if rev := dep.Annotations["deployment.kubernetes.io/revision"]; rev != "" {
				var n int64
				if _, err := fmt.Sscanf(rev, "%d", &n); err == nil && n >= min {
					return
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for Deployment revision >= %d", min)
}
