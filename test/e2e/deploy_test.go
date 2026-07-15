//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestDeployRedisOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)

	const name = "redis"
	// Clean previous run if any.
	_ = client.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{})
	_ = client.CoreV1().Services(ns).Delete(ctx, name, metav1.DeleteOptions{})

	var out bytes.Buffer
	cfg := config.Resolved{
		Provider:  "stub",
		Namespace: ns,
		Approve:   true,
		Prompt:    "deploy redis",
	}
	err := pipeline.RunWith(ctx, cfg, &out, pipeline.Deps{
		Provider: llm.DeployStub(name, ns, "", 1, 0),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("pipeline: %v\noutput:\n%s", err, out.String())
	}
	t.Log(out.String())

	dep, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(dep.Spec.Template.Spec.Containers) == 0 || dep.Spec.Template.Spec.Containers[0].Image != "redis:7-alpine" {
		t.Fatalf("unexpected deployment: %+v", dep.Spec.Template.Spec.Containers)
	}
	if _, err := client.CoreV1().Services(ns).Get(ctx, name, metav1.GetOptions{}); err != nil {
		t.Fatalf("expected service: %v", err)
	}
}
