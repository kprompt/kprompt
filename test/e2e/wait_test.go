//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestScaleWithWaitOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)
	ensureDeployment(t, ctx, client, 1)

	var out bytes.Buffer
	err := pipeline.RunWith(ctx, config.Resolved{
		Namespace: ns,
		Approve:   true,
		Wait:      true,
		Timeout:   2 * time.Minute,
		Prompt:    "scale demo to 2",
	}, &out, pipeline.Deps{
		Provider: llm.ScaleStub(deployName, ns, 2),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("pipeline: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "Waiting for Deployment/"+deployName) {
		t.Fatalf("expected wait output, got:\n%s", out.String())
	}
	if !strings.Contains(out.String(), "ready") {
		t.Fatalf("expected ready, got:\n%s", out.String())
	}
	t.Log(out.String())

	dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 2 {
		t.Fatalf("replicas=%v", dep.Spec.Replicas)
	}
}
