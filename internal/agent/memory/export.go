package memory

import (
	"errors"
	"sort"
	"time"
)

// KindExport is the fleet export document kind (RT-023).
const KindExport = "NamespaceMemoryExport"

// errNotListable is returned when a Store can't enumerate namespaces (RT-023).
var errNotListable = errors.New("memory: store does not support fleet export (no namespace listing)")

// Lister is an optional Store capability to enumerate stored namespaces (RT-023).
// Implemented by FileStore (scan dir) and ConfigMapStore (list labelled CMs).
type Lister interface {
	ListNamespaces() ([]string, error)
}

// ExportBundle is a local, offline fleet backup of namespace memory (RT-023).
// Never uploaded to api.kprompt.ai — written to a local file / stdout only.
type ExportBundle struct {
	APIVersion    string         `json:"apiVersion"`
	Kind          string         `json:"kind"`
	SchemaVersion string         `json:"schemaVersion"`
	GeneratedAt   time.Time      `json:"generatedAt"`
	Source        string         `json:"source"` // file | configmap
	Summary       ExportSummary  `json:"summary"`
	Namespaces    []Snapshot     `json:"namespaces"`
	Note          string         `json:"note"`
}

// ExportSummary aggregates a fleet export for a quick glance (RT-023).
type ExportSummary struct {
	Namespaces int            `json:"namespaces"`
	Facts      int            `json:"facts"`
	ByKind     map[string]int `json:"byKind,omitempty"`
}

// BuildExport wraps namespace snapshots into an offline fleet bundle (RT-023).
func BuildExport(source string, snaps []Snapshot) ExportBundle {
	byKind := map[string]int{}
	total := 0
	for _, s := range snaps {
		for _, f := range s.Facts {
			total++
			byKind[f.Kind]++
		}
	}
	sort.Slice(snaps, func(i, j int) bool { return snaps[i].Namespace < snaps[j].Namespace })
	return ExportBundle{
		APIVersion:    APIVersion,
		Kind:          KindExport,
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Source:        source,
		Summary: ExportSummary{
			Namespaces: len(snaps),
			Facts:      total,
			ByKind:     byKind,
		},
		Namespaces: snaps,
		Note:       "Offline namespace-memory backup — local file only, never uploaded to api.kprompt.ai (RT-023 / ADR-0022).",
	}
}

// ExportFleet loads every stored namespace via a Lister and builds a bundle (RT-023).
func ExportFleet(store Store, source string) (ExportBundle, error) {
	lister, ok := store.(Lister)
	if !ok {
		return ExportBundle{}, errNotListable
	}
	namespaces, err := lister.ListNamespaces()
	if err != nil {
		return ExportBundle{}, err
	}
	mem := New(store)
	var snaps []Snapshot
	for _, ns := range namespaces {
		snap, err := mem.List(ns)
		if err != nil {
			continue
		}
		snaps = append(snaps, snap)
	}
	return BuildExport(source, snaps), nil
}
