package memory

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// FileStore persists one JSON file per namespace under Dir.
type FileStore struct {
	Dir string
}

func (s FileStore) path(ns string) string {
	safe := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, ns)
	return filepath.Join(s.Dir, safe+".json")
}

// Load reads namespace facts from disk.
func (s FileStore) Load(namespace string) (Snapshot, error) {
	b, err := os.ReadFile(s.path(namespace))
	if err != nil {
		return Snapshot{}, err
	}
	return Decode(b)
}

// Save writes namespace facts to disk (0600).
func (s FileStore) Save(snap Snapshot) error {
	if err := os.MkdirAll(s.Dir, 0o700); err != nil {
		return err
	}
	b, err := Encode(snap)
	if err != nil {
		return err
	}
	tmp := s.path(snap.Namespace) + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path(snap.Namespace))
}

// ListNamespaces enumerates stored namespaces by scanning *.json under Dir (RT-023).
func (s FileStore) ListNamespaces() ([]string, error) {
	entries, err := os.ReadDir(s.Dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".tmp") {
			continue
		}
		ns := strings.TrimSuffix(name, ".json")
		if ns == "" {
			continue
		}
		out = append(out, ns)
	}
	return out, nil
}

// MemStore is an in-process store for tests.
type MemStore struct {
	mu   sync.Mutex
	data map[string]Snapshot
}

// NewMemStore returns an empty MemStore.
func NewMemStore() *MemStore {
	return &MemStore{data: map[string]Snapshot{}}
}

func (s *MemStore) Load(namespace string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.data[namespace]
	if !ok {
		return Snapshot{}, fmt.Errorf("memory: no facts for %s", namespace)
	}
	return snap, nil
}

// ListNamespaces enumerates stored namespaces (RT-023 test support).
func (s *MemStore) ListNamespaces() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.data))
	for ns := range s.data {
		out = append(out, ns)
	}
	return out, nil
}

func (s *MemStore) Save(snap Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string]Snapshot{}
	}
	// deep-ish copy via JSON
	b, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	var copy Snapshot
	if err := json.Unmarshal(b, &copy); err != nil {
		return err
	}
	s.data[snap.Namespace] = copy
	return nil
}
