package coordinator

import (
	"os"
	"path/filepath"
	"testing"

	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/agent/handoff"
)

func sampleSnapshot() Snapshot {
	return Snapshot{Records: []Record{{Envelope: handoff.Envelope{FromNamespace: "a", SuspectNamespace: "b"}}}}
}

func TestConfigMapStoreKnowledgeCreateUpdate(t *testing.T) {
	client := fake.NewSimpleClientset()
	cs := ConfigMapStore{Client: client, Namespace: "kprompt-system"}

	// Create branch.
	if err := cs.Save(sampleSnapshot()); err != nil {
		t.Fatalf("save create: %v", err)
	}
	got, err := cs.Load()
	if err != nil || len(got.Records) != 1 {
		t.Fatalf("load after create: %v %+v", err, got)
	}
	// Update branch (ConfigMap already exists).
	snap := sampleSnapshot()
	snap.Records = append(snap.Records, Record{Envelope: handoff.Envelope{FromNamespace: "c", SuspectNamespace: "d"}})
	if err := cs.Save(snap); err != nil {
		t.Fatalf("save update: %v", err)
	}
	got, _ = cs.Load()
	if len(got.Records) != 2 {
		t.Fatalf("expected 2 records after update, got %d", len(got.Records))
	}
}

func TestConfigMapStoreOutcomesCreateUpdate(t *testing.T) {
	client := fake.NewSimpleClientset()
	cs := ConfigMapStore{Client: client, Namespace: "kprompt-system"}

	snap := OutcomeSnapshot{Outcomes: []OutcomeRecord{{Namespace: "a", Action: "restartDeployment", Result: "apply_success"}}}
	if err := cs.SaveOutcomes(snap); err != nil {
		t.Fatalf("save outcomes create: %v", err)
	}
	got, err := cs.LoadOutcomes()
	if err != nil || len(got.Outcomes) != 1 {
		t.Fatalf("load outcomes: %v %+v", err, got)
	}
	snap.Outcomes = append(snap.Outcomes, OutcomeRecord{Namespace: "b", Action: "scaleDeployment", Result: "apply_failed"})
	if err := cs.SaveOutcomes(snap); err != nil {
		t.Fatalf("save outcomes update: %v", err)
	}
	got, _ = cs.LoadOutcomes()
	if len(got.Outcomes) != 2 {
		t.Fatalf("expected 2 outcomes after update, got %d", len(got.Outcomes))
	}
}

func TestFileStoreLoadBranches(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "handoffs.json")

	// Valid JSON roundtrip through Save then Load.
	if err := (FileStore{Path: path}).Save(sampleSnapshot()); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := (FileStore{Path: path}).Load()
	if err != nil || len(got.Records) != 1 {
		t.Fatalf("load valid: %v %+v", err, got)
	}

	// Empty file -> empty snapshot with schema default.
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte("   "), 0o600); err != nil {
		t.Fatal(err)
	}
	if snap, err := (FileStore{Path: empty}).Load(); err != nil || snap.SchemaVersion == "" {
		t.Fatalf("empty load: %v %+v", err, snap)
	}

	// Malformed JSON -> error.
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte("{not-json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileStore{Path: bad}).Load(); err == nil {
		t.Fatal("malformed load should error")
	}

	// Missing outcomes sibling -> empty ok.
	if snap, err := (FileStore{Path: path}).LoadOutcomes(); err != nil || snap.SchemaVersion == "" {
		t.Fatalf("missing outcomes load: %v %+v", err, snap)
	}
	// Malformed outcomes sibling -> error.
	if err := os.WriteFile(filepath.Join(dir, OutcomeFileName), []byte("{bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := (FileStore{Path: path}).LoadOutcomes(); err == nil {
		t.Fatal("malformed outcomes load should error")
	}
}
