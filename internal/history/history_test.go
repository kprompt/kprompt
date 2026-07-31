package history

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

func setupTempHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")
	return home
}

func writeHistoryEntries(t *testing.T, entries ...Entry) string {
	t.Helper()
	home := setupTempHome(t)
	for _, entry := range entries {
		if err := Append(entry); err != nil {
			t.Fatalf("append history entry %q: %v", entry.Prompt, err)
		}
	}
	path, err := DefaultPath()
	if err != nil {
		t.Fatalf("default path: %v", err)
	}
	if got := filepath.Dir(path); got != filepath.Join(home, ".kprompt") {
		t.Fatalf("history dir = %q, want %q", got, filepath.Join(home, ".kprompt"))
	}
	return path
}

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

func TestListFiltered(t *testing.T) {
	now := time.Now().UTC()
	writeHistoryEntries(t,
		Entry{Time: now.Add(-3 * time.Minute), Prompt: "list pods", Kind: "get", Namespace: "team-a", Summary: "List pods", Applied: true},
		Entry{Time: now.Add(-2 * time.Minute), Prompt: "scale api", Kind: "scale", Namespace: "team-b", Summary: "Scale api", Applied: false},
		Entry{Time: now.Add(-1 * time.Minute), Prompt: "deploy api", Kind: "deploy", Namespace: "team-a", Summary: "Deploy api", Applied: true},
	)

	t.Run("filters by namespace", func(t *testing.T) {
		entries, err := ListFiltered(10, FilterOptions{Namespace: "TEAM-A"})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 2 {
			t.Fatalf("len=%d", len(entries))
		}
		if entries[0].Prompt != "deploy api" || entries[1].Prompt != "list pods" {
			t.Fatalf("prompts=%q,%q", entries[0].Prompt, entries[1].Prompt)
		}
	})

	t.Run("filters by kind", func(t *testing.T) {
		entries, err := ListFiltered(10, FilterOptions{Kind: "DePlOy"})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("len=%d", len(entries))
		}
		if entries[0].Prompt != "deploy api" {
			t.Fatalf("prompt=%q", entries[0].Prompt)
		}
	})

	t.Run("combines namespace and kind", func(t *testing.T) {
		entries, err := ListFiltered(10, FilterOptions{Namespace: "team-a", Kind: "deploy"})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("len=%d", len(entries))
		}
		if entries[0].Prompt != "deploy api" {
			t.Fatalf("prompt=%q", entries[0].Prompt)
		}
	})

	t.Run("honors limit", func(t *testing.T) {
		entries, err := ListFiltered(1, FilterOptions{Namespace: "team-a"})
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) != 1 {
			t.Fatalf("len=%d", len(entries))
		}
		if entries[0].Prompt != "deploy api" {
			t.Fatalf("prompt=%q", entries[0].Prompt)
		}
	})
}

func TestClear(t *testing.T) {
	setupTempHome(t)
	if err := Append(Entry{Time: time.Now().UTC(), Prompt: "list pods", Kind: "get", Namespace: "team-a", Applied: true}); err != nil {
		t.Fatal(err)
	}
	if err := Append(Entry{Time: time.Now().UTC(), Prompt: "scale api", Kind: "scale", Namespace: "team-b", Applied: false}); err != nil {
		t.Fatal(err)
	}

	path, err := DefaultPath()
	if err != nil {
		t.Fatal(err)
	}
	if err := Clear(); err != nil {
		t.Fatal(err)
	}

	entries, err := List(10)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("len=%d", len(entries))
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(string(data))) != 0 {
		t.Fatalf("history file not cleared: %q", string(data))
	}
}
