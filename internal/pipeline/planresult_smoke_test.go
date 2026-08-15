package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/output"
)

// PlanResult smoke: CI-gateable JSON contract without a live LLM or apiserver.
// Complements intent gold_extract.jsonl (kind routing) with end-to-end PlanResult shape.

func TestPlanResultSmokeWipeJSONDenied(t *testing.T) {
	prompts := []string{
		"delete all pods",
		"delete everything in the cluster",
	}
	for _, prompt := range prompts {
		prompt := prompt
		t.Run(prompt, func(t *testing.T) {
			var out bytes.Buffer
			var got output.PlanResult
			err := RunWith(context.Background(), config.Resolved{
				Approve: true, // must not waive wipe
				Output:  "json",
				Prompt:  prompt,
			}, &out, Deps{
				// No Provider/Client — wipe returns before Extract.
				SkipOrgPolicy: true,
				OnResult:      func(doc output.PlanResult) { got = doc },
			})
			if err != nil {
				t.Fatal(err)
			}
			assertPlanResultJSON(t, out.Bytes(), &got)
			if !got.Risk.Denied {
				t.Fatalf("risk.denied=false: %+v", got.Risk)
			}
			if got.Risk.Level != "denied" {
				t.Fatalf("risk.level=%q", got.Risk.Level)
			}
			if got.Applied {
				t.Fatal("wipe must never set applied=true")
			}
			if got.Prompt != prompt {
				t.Fatalf("prompt=%q", got.Prompt)
			}
			if len(got.Plan.Actions) != 0 {
				t.Fatalf("wipe should not emit actions: %+v", got.Plan.Actions)
			}
		})
	}
}

func TestPlanResultSmokeScaleJSONNotApplied(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "staging", 1))
	var out bytes.Buffer
	var got output.PlanResult
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "staging",
		Output:    "json",
		Prompt:    "scale api to 3",
	}, &out, Deps{
		Provider:      llm.ScaleStub("api", "staging", 3),
		Client:        client,
		IsTerminal:    boolPtr(false),
		SkipOrgPolicy: true,
		OnResult:      func(doc output.PlanResult) { got = doc },
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPlanResultJSON(t, out.Bytes(), &got)

	if got.Risk.Denied {
		t.Fatalf("scale must not be denied: %+v", got.Risk)
	}
	if got.Applied {
		t.Fatal("plan-only run must set applied=false")
	}
	if got.Plan.Intent != "scale" {
		t.Fatalf("intent=%q", got.Plan.Intent)
	}
	if !got.Plan.RequiresApproval {
		t.Fatal("scale plan should requireApproval")
	}
	if got.Plan.Namespace != "staging" {
		t.Fatalf("namespace=%q", got.Plan.Namespace)
	}
	if len(got.Plan.Actions) == 0 {
		t.Fatal("expected scale action")
	}
	a := got.Plan.Actions[0]
	if a.Name != "api" || a.Namespace != "staging" {
		t.Fatalf("action target=%+v", a)
	}
	if a.Replicas == nil || *a.Replicas != 3 {
		t.Fatalf("replicas=%v", a.Replicas)
	}

	dep, err := client.AppsV1().Deployments("staging").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 1 {
		t.Fatalf("cluster must stay at 1 replica, got %v", dep.Spec.Replicas)
	}
}

func TestPlanResultSmokeScaleJSONAppliedWithApprove(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("api", "staging", 1))
	var out bytes.Buffer
	var got output.PlanResult
	err := RunWith(context.Background(), config.Resolved{
		Approve:   true,
		Namespace: "staging",
		Output:    "json",
		Prompt:    "scale api to 3",
	}, &out, Deps{
		Provider:      llm.ScaleStub("api", "staging", 3),
		Client:        client,
		SkipOrgPolicy: true,
		OnResult:      func(doc output.PlanResult) { got = doc },
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPlanResultJSON(t, out.Bytes(), &got)
	if got.Risk.Denied {
		t.Fatalf("scale must not be denied: %+v", got.Risk)
	}
	if !got.Applied {
		t.Fatal("--approve must set applied=true")
	}
	if got.Plan.Intent != "scale" {
		t.Fatalf("intent=%q", got.Plan.Intent)
	}
	dep, err := client.AppsV1().Deployments("staging").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if dep.Spec.Replicas == nil || *dep.Spec.Replicas != 3 {
		t.Fatalf("expected replicas=3 after approve, got %v", dep.Spec.Replicas)
	}
}

// TestPlanResultSmokeApproveDoesNotWaiveWipe documents the approve gate asymmetry:
// --approve applies gated mutates; it never waives wipe-class prompt denies.
func TestPlanResultSmokeApproveDoesNotWaiveWipe(t *testing.T) {
	var out bytes.Buffer
	var got output.PlanResult
	err := RunWith(context.Background(), config.Resolved{
		Approve: true,
		Output:  "json",
		Prompt:  "delete all pods",
	}, &out, Deps{
		SkipOrgPolicy: true,
		OnResult:      func(doc output.PlanResult) { got = doc },
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPlanResultJSON(t, out.Bytes(), &got)
	if !got.Risk.Denied || got.Applied {
		t.Fatalf("wipe+approve must stay denied/applied=false: denied=%v applied=%v", got.Risk.Denied, got.Applied)
	}
}

func TestPlanResultSmokeNamedDeleteJSONHighRiskNotApplied(t *testing.T) {
	client := fake.NewSimpleClientset(deployment("redis", "default", 1))
	var out bytes.Buffer
	var got output.PlanResult
	err := RunWith(context.Background(), config.Resolved{
		Approve:   false,
		Namespace: "default",
		Output:    "json",
		Prompt:    "delete deployment redis",
	}, &out, Deps{
		Provider:      llm.DeleteStub("redis", "default", "Deployment"),
		Client:        client,
		IsTerminal:    boolPtr(false),
		SkipOrgPolicy: true,
		OnResult:      func(doc output.PlanResult) { got = doc },
	})
	if err != nil {
		t.Fatal(err)
	}
	assertPlanResultJSON(t, out.Bytes(), &got)
	if got.Risk.Denied {
		t.Fatalf("named delete must pass prompt+plan safety: %+v", got.Risk)
	}
	if got.Applied {
		t.Fatal("named delete without approve must not apply")
	}
	if got.Plan.Intent != "delete" {
		t.Fatalf("intent=%q", got.Plan.Intent)
	}
	if _, err := client.AppsV1().Deployments("default").Get(context.Background(), "redis", metav1.GetOptions{}); err != nil {
		t.Fatalf("deployment deleted without approve: %v", err)
	}
}

func assertPlanResultJSON(t *testing.T, raw []byte, viaOnResult *output.PlanResult) {
	t.Helper()
	if !json.Valid(raw) {
		t.Fatalf("invalid json: %s", raw)
	}
	var doc output.PlanResult
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, raw)
	}
	if doc.APIVersion != output.APIVersion {
		t.Fatalf("apiVersion=%q", doc.APIVersion)
	}
	if doc.Kind != output.KindPlanResult {
		t.Fatalf("kind=%q", doc.Kind)
	}
	if doc.SchemaVersion != output.SchemaVersion {
		t.Fatalf("schemaVersion=%q", doc.SchemaVersion)
	}
	if viaOnResult != nil {
		if viaOnResult.Risk.Denied != doc.Risk.Denied || viaOnResult.Applied != doc.Applied {
			t.Fatalf("OnResult mismatch stdout: onResult denied=%v applied=%v stdout denied=%v applied=%v",
				viaOnResult.Risk.Denied, viaOnResult.Applied, doc.Risk.Denied, doc.Applied)
		}
	}
	*viaOnResult = doc
}
