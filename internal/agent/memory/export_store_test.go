package memory

import (
	"os"
	"path/filepath"
	"testing"
)

// stubStore implements Store but not Lister, to exercise the not-listable path.
type stubStore struct{}

func (stubStore) Load(string) (Snapshot, error) { return Snapshot{}, nil }
func (stubStore) Save(Snapshot) error           { return nil }

func TestFileStoreListNamespacesScan(t *testing.T) {
	// Non-existent directory returns an empty list without error.
	missing := FileStore{Dir: filepath.Join(t.TempDir(), "nope")}
	if ns, err := missing.ListNamespaces(); err != nil || ns != nil {
		t.Fatalf("missing dir: ns=%v err=%v", ns, err)
	}

	dir := t.TempDir()
	fs := FileStore{Dir: dir}
	if err := fs.Save(Snapshot{Namespace: "payments", Facts: []Fact{{Key: "redis", Kind: KindDependency}}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	// Noise the scanner must ignore: a .tmp file, a bare ".json", and a subdir.
	if err := os.WriteFile(filepath.Join(dir, "leftover.tmp"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".json"), []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	ns, err := fs.ListNamespaces()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(ns) != 1 || ns[0] != "payments" {
		t.Fatalf("expected only payments, got %v", ns)
	}
}

func TestExportFleet(t *testing.T) {
	// A store without namespace listing is not fleet-exportable.
	if _, err := ExportFleet(stubStore{}, "test"); err == nil {
		t.Fatal("expected not-listable error")
	}

	dir := t.TempDir()
	fs := FileStore{Dir: dir}
	if err := fs.Save(Snapshot{Namespace: "payments", Facts: []Fact{{Key: "redis", Kind: KindDependency}}}); err != nil {
		t.Fatalf("save: %v", err)
	}
	bundle, err := ExportFleet(fs, "unit-test")
	if err != nil {
		t.Fatalf("export fleet: %v", err)
	}
	if bundle.Summary.Namespaces != 1 || bundle.Source != "unit-test" {
		t.Fatalf("unexpected bundle: %+v", bundle.Summary)
	}
}
