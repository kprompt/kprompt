// Package patterns learns recurring incident signatures for “seen before”
// confidence boosts (AG-016). Never mutates the cluster from a match.
package patterns

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/incident"
)

const (
	KindSnapshot  = "IncidentPatterns"
	APIVersion    = "kprompt.io/v1"
	SchemaVersion = "1"

	ConfigMapName = "kprompt-incident-patterns"
	ConfigMapKey  = "patterns.json"

	// MaxBoost is the maximum confidence increase from a match (Observe-only).
	MaxBoost = 0.15
	// MinPriorCount before a pattern is considered “seen before”.
	MinPriorCount = 2
)

// Pattern is a durable signature of similar past incidents.
type Pattern struct {
	ID             string    `json:"id"`
	Signature      string    `json:"signature"`
	Namespace      string    `json:"namespace"`
	Count          int       `json:"count"`
	Confirmed      int       `json:"confirmed,omitempty"`      // AG-033: resolved outcomes
	FalsePositives int       `json:"falsePositives,omitempty"` // AG-033: FP outcomes
	Weight         float64   `json:"weight,omitempty"`         // AG-033: boost multiplier (default 1)
	LastSummary    string    `json:"lastSummary,omitempty"`
	LastRootCause  string    `json:"lastRootCause,omitempty"`
	LastRec        string    `json:"lastRecommendation,omitempty"`
	LastSeverity   string    `json:"lastSeverity,omitempty"`
	LastSeenAt     time.Time `json:"lastSeenAt"`
	ExampleReasons []string  `json:"exampleReasons,omitempty"`
}

// Outcome is a post-notify learning signal (AG-033).
type Outcome string

const (
	OutcomeResolved      Outcome = "resolved"
	OutcomeFalsePositive Outcome = "false_positive"
)

// Snapshot is the persisted document.
type Snapshot struct {
	APIVersion    string    `json:"apiVersion"`
	Kind          string    `json:"kind"`
	SchemaVersion string    `json:"schemaVersion"`
	Namespace     string    `json:"namespace"`
	Patterns      []Pattern `json:"patterns"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Store loads/saves pattern snapshots.
type Store interface {
	Load(namespace string) (Snapshot, error)
	Save(snap Snapshot) error
}

// Library is a thread-safe pattern matcher + recorder.
type Library struct {
	mu    sync.Mutex
	store Store
}

// New wraps a Store.
func New(store Store) *Library {
	return &Library{store: store}
}

// List returns the persisted pattern snapshot for a namespace (AG-054).
func (l *Library) List(namespace string) (Snapshot, error) {
	if l == nil || l.store == nil {
		return Snapshot{}, fmt.Errorf("patterns: library unset")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(namespace)
	if err != nil {
		return emptySnapshot(namespace), nil
	}
	return snap, nil
}

// Match finds the best prior pattern for this context (excluding the current incident id noise).
func (l *Library) Match(namespace string, agentCtx ctxbuild.AgentContext) (Pattern, bool) {
	if l == nil || l.store == nil {
		return Pattern{}, false
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(namespace)
	if err != nil {
		return Pattern{}, false
	}
	sig := Signature(agentCtx)
	for _, p := range snap.Patterns {
		if p.Signature == sig && p.Count >= MinPriorCount {
			return p, true
		}
	}
	// Soft match: same primary reason tokens
	soft := softSignature(agentCtx)
	var best Pattern
	found := false
	for _, p := range snap.Patterns {
		if p.Count < MinPriorCount {
			continue
		}
		if soft != "" && strings.HasPrefix(p.Signature, soft) {
			if !found || p.Count > best.Count {
				best = p
				found = true
			}
		}
	}
	return best, found
}

// Record upserts a pattern from a gated analysis result (Observe learning only).
func (l *Library) Record(namespace string, agentCtx ctxbuild.AgentContext, severity, summary, root, rec string) (Pattern, error) {
	if l == nil || l.store == nil {
		return Pattern{}, fmt.Errorf("patterns: library unset")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(namespace)
	if err != nil {
		snap = emptySnapshot(namespace)
	}
	sig := Signature(agentCtx)
	now := time.Now().UTC()
	idx := -1
	for i, p := range snap.Patterns {
		if p.Signature == sig {
			idx = i
			break
		}
	}
	reasons := topReasons(agentCtx, 5)
	if idx >= 0 {
		p := snap.Patterns[idx]
		p.Count++
		p.LastSummary = summary
		p.LastRootCause = root
		p.LastRec = rec
		p.LastSeverity = severity
		p.LastSeenAt = now
		p.ExampleReasons = mergeReasons(p.ExampleReasons, reasons)
		snap.Patterns[idx] = p
	} else {
		snap.Patterns = append(snap.Patterns, Pattern{
			ID:             "pattern/" + sig[:12],
			Signature:      sig,
			Namespace:      namespace,
			Count:          1,
			LastSummary:    summary,
			LastRootCause:  root,
			LastRec:        rec,
			LastSeverity:   severity,
			LastSeenAt:     now,
			ExampleReasons: reasons,
		})
	}
	snap.UpdatedAt = now
	snap.Namespace = namespace
	sort.Slice(snap.Patterns, func(i, j int) bool {
		return snap.Patterns[i].Count > snap.Patterns[j].Count
	})
	// Cap history to keep files small.
	if len(snap.Patterns) > 200 {
		snap.Patterns = snap.Patterns[:200]
	}
	if err := l.store.Save(snap); err != nil {
		return Pattern{}, err
	}
	for _, p := range snap.Patterns {
		if p.Signature == sig {
			return p, nil
		}
	}
	return Pattern{}, nil
}

// RecordOutcome updates pattern weights from resolve / false-positive feedback (AG-033).
// Does not increment Count (occurrence) — outcomes are separate from fire learning.
func (l *Library) RecordOutcome(namespace string, agentCtx ctxbuild.AgentContext, outcome Outcome) (Pattern, error) {
	if l == nil || l.store == nil {
		return Pattern{}, fmt.Errorf("patterns: library unset")
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	snap, err := l.store.Load(namespace)
	if err != nil {
		return Pattern{}, err
	}
	sig := Signature(agentCtx)
	idx := -1
	for i, p := range snap.Patterns {
		if p.Signature == sig {
			idx = i
			break
		}
	}
	if idx < 0 {
		return Pattern{}, fmt.Errorf("patterns: no pattern for signature")
	}
	p := snap.Patterns[idx]
	if p.Weight <= 0 {
		p.Weight = 1
	}
	switch outcome {
	case OutcomeResolved:
		p.Confirmed++
		p.Weight = clamp01(p.Weight + 0.05)
	case OutcomeFalsePositive:
		p.FalsePositives++
		p.Weight = clamp01(p.Weight - 0.15)
		if p.Weight < 0.2 {
			p.Weight = 0.2
		}
	default:
		return Pattern{}, fmt.Errorf("patterns: unknown outcome %q", outcome)
	}
	p.LastSeenAt = time.Now().UTC()
	snap.Patterns[idx] = p
	snap.UpdatedAt = p.LastSeenAt
	if err := l.store.Save(snap); err != nil {
		return Pattern{}, err
	}
	return p, nil
}

// EffectiveBoost returns the confidence boost for a match after AG-033 weight.
func EffectiveBoost(match Pattern) float64 {
	if match.Count < MinPriorCount {
		return 0
	}
	base := 0.10
	if match.Count >= 5 {
		base = MaxBoost
	}
	w := match.Weight
	if w <= 0 {
		w = 1
	}
	// Heavy FP history dampens boost.
	if match.FalsePositives > match.Confirmed && match.FalsePositives >= 2 {
		w = minFloat(w, 0.4)
	}
	boost := base * w
	if boost > MaxBoost {
		boost = MaxBoost
	}
	if boost < 0 {
		return 0
	}
	return boost
}

// ApplyBoost raises confidence and annotates root cause with “seen before”.
// Never changes recommendation into an apply/mutate action.
func ApplyBoost(res SeverityConfidence, match Pattern) (SeverityConfidence, string) {
	boost := EffectiveBoost(match)
	if boost <= 0 {
		return res, ""
	}
	res.Confidence = clamp01(res.Confidence + boost)
	note := fmt.Sprintf("Seen before (%d×): %s", match.Count, firstNonEmpty(match.LastRootCause, match.LastSummary))
	if match.FalsePositives > 0 {
		note += fmt.Sprintf(" [fp=%d confirmed=%d]", match.FalsePositives, match.Confirmed)
	}
	if strings.TrimSpace(res.RootCause) == "" || res.RootCause == "Unknown" {
		res.RootCause = firstNonEmpty(match.LastRootCause, res.RootCause)
	}
	if match.LastRec != "" && !looksLikeMutate(match.LastRec) {
		if strings.Contains(strings.ToLower(res.Recommendation), "investigate") ||
			strings.Contains(strings.ToLower(res.Recommendation), "inspect") {
			res.Recommendation = match.LastRec
		}
	}
	res.RootCause = strings.TrimSpace(res.RootCause + "; " + note)
	return res, note
}

// SeverityConfidence is the mutable slice of an analysis result we may boost.
type SeverityConfidence struct {
	Confidence     float64
	RootCause      string
	Recommendation string
}

func looksLikeMutate(s string) bool {
	lower := strings.ToLower(s)
	for _, bad := range []string{"kubectl apply", "kubectl delete", "kubectl patch", "helm upgrade", "auto-remediat", "rolling restart now"} {
		if strings.Contains(lower, bad) {
			return true
		}
	}
	return false
}

// Signature builds a stable key from workload kind + dominant event reasons.
func Signature(agentCtx ctxbuild.AgentContext) string {
	soft := softSignature(agentCtx)
	workload := "unknown"
	if agentCtx.Target != nil && agentCtx.Target.Kind != "" {
		workload = strings.ToLower(agentCtx.Target.Kind)
	} else if agentCtx.Incident.PrimaryResource != nil {
		workload = strings.ToLower(agentCtx.Incident.PrimaryResource.Kind)
	}
	raw := soft + "|" + workload + "|" + classifyBucket(agentCtx)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func softSignature(agentCtx ctxbuild.AgentContext) string {
	reasons := topReasons(agentCtx, 3)
	return strings.Join(reasons, "+")
}

func classifyBucket(agentCtx ctxbuild.AgentContext) string {
	blob := strings.ToLower(agentCtx.Incident.Summary + " " + joinEv(agentCtx.Incident.Evidence) + " " + joinEv(agentCtx.LogSnippets))
	switch {
	case strings.Contains(blob, "oom"):
		return "oom"
	case strings.Contains(blob, "crashloop"), strings.Contains(blob, "backoff"):
		return "crashloop"
	case strings.Contains(blob, "imagepull"), strings.Contains(blob, "errimage"):
		return "imagepull"
	case strings.Contains(blob, "failedscheduling"), strings.Contains(blob, "pending"):
		return "scheduling"
	case strings.Contains(blob, "unhealthy"), strings.Contains(blob, "probe"):
		return "probe"
	default:
		return "other"
	}
}

func topReasons(agentCtx ctxbuild.AgentContext, n int) []string {
	counts := map[string]int{}
	for _, e := range agentCtx.Incident.Evidence {
		r := normalizeReason(e.Reason)
		if r != "" {
			counts[r]++
		}
	}
	for _, e := range agentCtx.RecentEvents {
		r := normalizeReason(e.Reason)
		if r != "" {
			counts[r]++
		}
	}
	type kv struct {
		k string
		v int
	}
	var list []kv
	for k, v := range counts {
		list = append(list, kv{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].v != list[j].v {
			return list[i].v > list[j].v
		}
		return list[i].k < list[j].k
	})
	var out []string
	for i := 0; i < len(list) && i < n; i++ {
		out = append(out, list[i].k)
	}
	if len(out) == 0 {
		out = append(out, classifyBucket(agentCtx))
	}
	return out
}

func normalizeReason(r string) string {
	r = strings.ToLower(strings.TrimSpace(r))
	r = strings.ReplaceAll(r, " ", "")
	return r
}

func mergeReasons(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, x := range append(a, b...) {
		if x == "" || seen[x] {
			continue
		}
		seen[x] = true
		out = append(out, x)
		if len(out) >= 8 {
			break
		}
	}
	return out
}

func joinEv(ev []incident.EvidenceRef) string {
	var b strings.Builder
	for _, e := range ev {
		b.WriteString(e.Reason)
		b.WriteByte(' ')
		b.WriteString(e.Message)
		b.WriteByte(' ')
	}
	return b.String()
}

func emptySnapshot(ns string) Snapshot {
	return Snapshot{
		APIVersion:    APIVersion,
		Kind:          KindSnapshot,
		SchemaVersion: SchemaVersion,
		Namespace:     ns,
		UpdatedAt:     time.Now().UTC(),
	}
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// Encode / Decode for file + ConfigMap payloads.
func Encode(snap Snapshot) ([]byte, error) {
	snap.APIVersion = APIVersion
	snap.Kind = KindSnapshot
	snap.SchemaVersion = SchemaVersion
	return json.MarshalIndent(snap, "", "  ")
}

func Decode(b []byte) (Snapshot, error) {
	var snap Snapshot
	if err := json.Unmarshal(b, &snap); err != nil {
		return Snapshot{}, err
	}
	return snap, nil
}

// DefaultDir returns ~/.config/kprompt/patterns (or KPROMPT_PATTERNS_DIR).
func DefaultDir() string {
	if d := strings.TrimSpace(os.Getenv("KPROMPT_PATTERNS_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".kprompt-patterns")
	}
	return filepath.Join(home, ".config", "kprompt", "patterns")
}

// FileStore persists one JSON file per namespace.
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

func (s FileStore) Load(namespace string) (Snapshot, error) {
	b, err := os.ReadFile(s.path(namespace))
	if err != nil {
		return Snapshot{}, err
	}
	return Decode(b)
}

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

// MemStore is an in-process store for tests.
type MemStore struct {
	mu   sync.Mutex
	data map[string]Snapshot
}

func NewMemStore() *MemStore {
	return &MemStore{data: map[string]Snapshot{}}
}

func (s *MemStore) Load(namespace string) (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	snap, ok := s.data[namespace]
	if !ok {
		return Snapshot{}, fmt.Errorf("patterns: empty")
	}
	return snap, nil
}

func (s *MemStore) Save(snap Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data == nil {
		s.data = map[string]Snapshot{}
	}
	b, _ := json.Marshal(snap)
	var copy Snapshot
	_ = json.Unmarshal(b, &copy)
	s.data[snap.Namespace] = copy
	return nil
}
