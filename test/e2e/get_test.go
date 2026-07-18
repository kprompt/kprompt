//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestListDeploymentsOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)
	ensureDeployment(t, ctx, client, 1)

	deps := pipelineKubeDeps(t, kubeconfig)
	deps.Provider = llm.GetStub("Deployment", "", ns, "")

	var out bytes.Buffer
	cfg := config.Resolved{
		Provider:  "stub",
		Namespace: ns,
		Approve:   false, // read-only must work without --approve
		Prompt:    "list deployments",
	}
	err := pipeline.RunWith(ctx, cfg, &out, deps)
	if err != nil {
		t.Fatalf("pipeline: %v\noutput:\n%s", err, out.String())
	}
	text := out.String()
	t.Log(text)
	if !strings.Contains(text, deployName) {
		t.Fatalf("expected deployment %q in output", deployName)
	}
	if !strings.Contains(text, "NAMESPACE") {
		t.Fatal("expected table header")
	}
}
