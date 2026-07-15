package history

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

func TestAppendAndList(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "history.jsonl")

	e1 := Entry{Time: time.Now().UTC(), Prompt: "list pods", Kind: "get", Summary: "List Pods", Applied: true}
	e2 := Entry{Time: time.Now().UTC(), Prompt: "scale api to 3", Kind: "scale", Summary: "Scale", Applied: false}
	if err := AppendPath(path, e1); err != nil {
		t.Fatal(err)
	}
	if err := AppendPath(path, e2); err != nil {
		t.Fatal(err)
	}
	list, err := ListPath(path, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("len=%d", len(list))
	}
	if list[0].Prompt != "scale api to 3" {
		t.Fatalf("newest=%q", list[0].Prompt)
	}
	if list[1].Prompt != "list pods" {
		t.Fatalf("older=%q", list[1].Prompt)
	}
}

func TestFromPlanOmitsManifest(t *testing.T) {
	e := FromPlan("deploy redis", "ctx", planner.ExecutionPlan{
		Intent:  intent.Intent{Kind: intent.KindDeploy, Target: intent.Target{Namespace: "demo"}},
		Summary: "Deploy redis",
		Actions: []planner.Action{{
			Op:       planner.OpCreate,
			Object:   planner.ObjectRef{Kind: "Deployment", Name: "redis", Namespace: "demo"},
			Manifest: "apiVersion: apps/v1\nsecret: SHOULD_NOT_APPEAR\n",
		}},
	}, safety.Result{Risk: safety.RiskMedium}, true)
	raw, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "SHOULD_NOT_APPEAR") || strings.Contains(string(raw), "apiVersion") {
		t.Fatalf("leaked manifest: %s", raw)
	}
	if len(e.Actions) != 1 || e.Actions[0] != "create Deployment/redis -n demo" {
		t.Fatalf("actions=%v", e.Actions)
	}
}

func TestFormatListEmpty(t *testing.T) {
	if FormatList(nil) != "No history yet.\n" {
		t.Fatal(FormatList(nil))
	}
}
