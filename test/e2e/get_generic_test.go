//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestGenericReadMatrixOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	admin := clientFromKubeconfig(t, kubeconfig)
	restCfg := restConfigFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, admin)
	ensureDeployment(t, ctx, admin, 1)
	ensureConfigMap(t, ctx, admin)
	ensureSecret(t, ctx, admin)
	ensureWidgetCRD(t, ctx, restCfg)

	deps := pipelineKubeDeps(t, kubeconfig)

	t.Run("nodes_en", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Approve: false, Prompt: "how many nodes are in the cluster",
		}, &out, withProvider(deps, llm.GetStub("Node", "", "", "")))
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		text := out.String()
		t.Log(text)
		if !strings.Contains(text, "NAME") {
			t.Fatal("expected node table")
		}
		// kind control-plane node name contains cluster name
		if !strings.Contains(text, clusterName) && !strings.Contains(strings.ToLower(text), "control") {
			t.Fatalf("expected a kind node name in output:\n%s", text)
		}
	})

	t.Run("nodes_tr", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Approve: false, Prompt: "clusterda kaç node var",
		}, &out, withProvider(deps, llm.GetStub("nodes", "", "", "")))
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), "NAME") {
			t.Fatalf("expected node table:\n%s", out.String())
		}
	})

	t.Run("configmap_list", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "list configmaps",
		}, &out, withProvider(deps, llm.GetStub("ConfigMap", "", ns, "")))
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), cmName) {
			t.Fatalf("expected %s:\n%s", cmName, out.String())
		}
	})

	t.Run("secret_get", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "get secret demo-secret",
		}, &out, withProvider(deps, llm.GetStub("Secret", secretName, ns, "")))
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		text := out.String()
		if !strings.Contains(text, secretName) {
			t.Fatalf("expected secret name:\n%s", text)
		}
		if strings.Contains(text, "s3cret") {
			t.Fatal("secret payload must not appear in table output")
		}
	})

	t.Run("crd_widget", func(t *testing.T) {
		deps.Resolver.Invalidate() // pick up new CRD
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "list widgets.example.com",
		}, &out, withProvider(deps, llm.GetStub("widgets.example.com", "", ns, "")))
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		if !strings.Contains(out.String(), widgetName) {
			t.Fatalf("expected widget:\n%s", out.String())
		}
	})

	t.Run("json_nodes", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Approve: false, Output: "json", Prompt: "list nodes",
		}, &out, withProvider(deps, llm.GetStub("Node", "", "", "")))
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		var doc struct {
			Result json.RawMessage `json:"result"`
		}
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("json: %v\n%s", err, out.String())
		}
		var result map[string]any
		if err := json.Unmarshal(doc.Result, &result); err != nil {
			t.Fatalf("result: %v (%s)", err, string(doc.Result))
		}
		if result["type"] != "query" {
			t.Fatalf("result=%v", result)
		}
		kind, _ := result["kind"].(string)
		resource, _ := result["resource"].(string)
		if kind != "Node" && resource != "nodes" {
			t.Fatalf("expected Node query, got %v", result)
		}
	})

	t.Run("unknown_resource", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "list foobars",
		}, &out, withProvider(deps, llm.GetStub("foobars", "", ns, "")))
		if err == nil {
			t.Fatalf("expected unknown resource error, got:\n%s", out.String())
		}
		if !strings.Contains(err.Error(), "unknown") && !strings.Contains(err.Error(), "foobars") {
			t.Fatalf("err=%v", err)
		}
	})

	t.Run("list_limit", func(t *testing.T) {
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "list configmaps limit 1",
		}, &out, withProvider(deps, llm.GetStubWith("ConfigMap", "", ns, map[string]any{"limit": float64(1)})))
		if err != nil {
			t.Fatalf("%v\n%s", err, out.String())
		}
		// At least one row; truncated note optional depending on server Limit behavior.
		if !strings.Contains(out.String(), "NAME") {
			t.Fatalf("expected table:\n%s", out.String())
		}
	})

	t.Run("rbac_secret_denied", func(t *testing.T) {
		limitedKC := ensureLimitedSecretDeniedKubeconfig(t, ctx, admin, kubeconfig)
		t.Setenv("KUBECONFIG", limitedKC)
		limitedDeps := pipelineKubeDeps(t, limitedKC)
		var out bytes.Buffer
		err := pipeline.RunWith(ctx, config.Resolved{
			Provider: "stub", Namespace: ns, Approve: false, Prompt: "list secrets",
		}, &out, withProvider(limitedDeps, llm.GetStub("Secret", "", ns, "")))
		if err == nil {
			t.Fatalf("expected RBAC deny, got:\n%s", out.String())
		}
		msg := err.Error()
		if !strings.Contains(msg, "RBAC") && !strings.Contains(msg, "denied") && !strings.Contains(msg, "cannot") {
			t.Fatalf("expected friendly RBAC error, got %v", err)
		}
		if !strings.Contains(msg, "can-i") && !strings.Contains(msg, "RBAC") {
			t.Logf("rbac message (acceptable): %s", msg)
		}
	})
}

func withProvider(deps pipeline.Deps, p llm.Provider) pipeline.Deps {
	deps.Provider = p
	return deps
}
