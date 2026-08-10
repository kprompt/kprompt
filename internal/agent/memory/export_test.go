package memory

import (
	"path/filepath"
	"testing"
)

func TestExportFleetMemStore(t *testing.T) {
	store := NewMemStore()
	mem := New(store)
	if _, err := mem.Upsert("payments", Fact{Kind: KindDependency, Key: "redis"}); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.Upsert("web", Fact{Kind: KindNote, Key: "team", Value: "core"}, Fact{Kind: KindDependency, Key: "postgres"}); err != nil {
		t.Fatal(err)
	}

	bundle, err := ExportFleet(store, "file")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Kind != KindExport {
		t.Fatalf("kind=%s", bundle.Kind)
	}
	if bundle.Summary.Namespaces != 2 {
		t.Fatalf("namespaces=%d", bundle.Summary.Namespaces)
	}
	if bundle.Summary.Facts != 3 {
		t.Fatalf("facts=%d", bundle.Summary.Facts)
	}
	if bundle.Summary.ByKind[KindDependency] != 2 || bundle.Summary.ByKind[KindNote] != 1 {
		t.Fatalf("byKind=%+v", bundle.Summary.ByKind)
	}
	// Deterministic namespace ordering.
	if bundle.Namespaces[0].Namespace != "payments" || bundle.Namespaces[1].Namespace != "web" {
		t.Fatalf("order=%+v", bundle.Namespaces)
	}
	if bundle.Note == "" {
		t.Fatal("want offline note")
	}
}

func TestExportFleetFileStore(t *testing.T) {
	dir := t.TempDir()
	store := FileStore{Dir: dir}
	if _, err := New(store).Upsert("payments", Fact{Kind: KindDependency, Key: "redis"}); err != nil {
		t.Fatal(err)
	}
	nss, err := store.ListNamespaces()
	if err != nil {
		t.Fatal(err)
	}
	if len(nss) != 1 || nss[0] != "payments" {
		t.Fatalf("list=%v", nss)
	}
	bundle, err := ExportFleet(store, "file")
	if err != nil {
		t.Fatal(err)
	}
	if bundle.Summary.Facts != 1 {
		t.Fatalf("facts=%d", bundle.Summary.Facts)
	}
	// .tmp files must be ignored by the lister.
	if _, err := filepath.Glob(filepath.Join(dir, "*.tmp")); err != nil {
		t.Fatal(err)
	}
}
