package coordinator

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	// DefaultOutcomeMaxKeep caps the durable cross-ns outcome ring (RT-021).
	DefaultOutcomeMaxKeep = 200
	// DefaultOutcomeTTL prunes outcomes older than this window (RT-021). 0 → no TTL.
	DefaultOutcomeTTL = 720 * time.Hour // 30 days
	kindOutcomeSummary = "CoordinatorOutcomeSummary"
)

// OutcomeRecord is one cross-namespace remediation outcome (RT-021).
// action + namespace + result only — never Secret values or full manifests.
type OutcomeRecord struct {
	Namespace string    `json:"namespace"`
	Action    string    `json:"action"`
	Result    string    `json:"result"` // apply_success | apply_failed | apply_partial | resolved | false_positive
	ActionID  string    `json:"actionId,omitempty"`
	IncidentID string   `json:"incidentId,omitempty"`
	At        time.Time `json:"at"`
}

// OutcomeSummary aggregates the outcome ring for evidence-not-proof reads (RT-022).
type OutcomeSummary struct {
	APIVersion    string                    `json:"apiVersion"`
	Kind          string                    `json:"kind"`
	SchemaVersion string                    `json:"schemaVersion"`
	GeneratedAt   time.Time                 `json:"generatedAt"`
	Total         int                       `json:"total"`
	Durable       bool                      `json:"durable"`
	ByResult      map[string]int            `json:"byResult,omitempty"`
	ByNamespace   map[string]int            `json:"byNamespace,omitempty"`
	ByAction      []OutcomeActionStat       `json:"byAction,omitempty"`
	Recent        []OutcomeRecord           `json:"recent,omitempty"`
	Note          string                    `json:"note"`
}

// OutcomeActionStat is per-action success/fail counts for fleet bias (RT-022).
type OutcomeActionStat struct {
	Action    string `json:"action"`
	Namespace string `json:"namespace,omitempty"`
	Success   int    `json:"success"`
	Failed    int    `json:"failed"`
	Partial   int    `json:"partial"`
	Total     int    `json:"total"`
}

// RecordOutcome appends a cross-ns outcome, prunes (TTL + cap), and persists (RT-021).
// Never mutates the cluster. Empty namespace/action/result is rejected.
func (s *Service) RecordOutcome(rec OutcomeRecord) error {
	if s == nil {
		return fmt.Errorf("coordinator: nil service")
	}
	rec.Namespace = strings.TrimSpace(rec.Namespace)
	rec.Action = strings.TrimSpace(rec.Action)
	rec.Result = strings.TrimSpace(rec.Result)
	if rec.Namespace == "" || rec.Action == "" || rec.Result == "" {
		return fmt.Errorf("coordinator outcome: namespace, action, result required")
	}
	if rec.At.IsZero() {
		rec.At = time.Now().UTC()
	} else {
		rec.At = rec.At.UTC()
	}
	s.mu.Lock()
	s.outcomes = append(s.outcomes, rec)
	s.pruneOutcomesLocked()
	snap := OutcomeSnapshot{SchemaVersion: SchemaVersion, Outcomes: append([]OutcomeRecord(nil), s.outcomes...)}
	store := s.outcomeStore()
	logf := s.PersistErrLog
	s.mu.Unlock()
	if store != nil {
		if err := store.SaveOutcomes(snap); err != nil && logf != nil {
			logf(err)
		}
	}
	return nil
}

// Outcomes returns a copy of the current outcome ring (newest last).
func (s *Service) Outcomes() []OutcomeRecord {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]OutcomeRecord, len(s.outcomes))
	copy(out, s.outcomes)
	return out
}

// RestoreOutcomes loads + prunes the durable outcome ring (RT-021).
func (s *Service) RestoreOutcomes() error {
	if s == nil {
		return nil
	}
	store := s.outcomeStore()
	if store == nil {
		return nil
	}
	snap, err := store.LoadOutcomes()
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.outcomes = append([]OutcomeRecord(nil), snap.Outcomes...)
	s.pruneOutcomesLocked()
	s.mu.Unlock()
	return nil
}

// pruneOutcomesLocked drops expired (TTL) then oldest-over-cap entries. Caller holds mu.
func (s *Service) pruneOutcomesLocked() {
	ttl := s.outcomeTTL
	if ttl <= 0 {
		ttl = DefaultOutcomeTTL
	}
	cap := s.outcomeMaxKeep
	if cap <= 0 {
		cap = DefaultOutcomeMaxKeep
	}
	if ttl > 0 {
		cutoff := time.Now().UTC().Add(-ttl)
		kept := s.outcomes[:0]
		for _, o := range s.outcomes {
			if o.At.After(cutoff) || o.At.Equal(cutoff) {
				kept = append(kept, o)
			}
		}
		s.outcomes = kept
	}
	if len(s.outcomes) > cap {
		s.outcomes = s.outcomes[len(s.outcomes)-cap:]
	}
}

func (s *Service) outcomeStore() OutcomeStore {
	if s == nil {
		return nil
	}
	if s.OutcomeStore != nil {
		return s.OutcomeStore
	}
	if os, ok := s.Store.(OutcomeStore); ok {
		return os
	}
	return nil
}

// OutcomeSummarize builds an evidence-not-proof summary of the outcome ring (RT-022).
func (s *Service) OutcomeSummarize() OutcomeSummary {
	recs := s.Outcomes()
	sum := OutcomeSummary{
		APIVersion:    APIVersion,
		Kind:          kindOutcomeSummary,
		SchemaVersion: SchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Total:         len(recs),
		Durable:       s.outcomeStore() != nil,
		ByResult:      map[string]int{},
		ByNamespace:   map[string]int{},
		Note:          "Coordinator outcome ring: cross-ns action/result bias only — evidence, not proof (AG-034 / RT-022). Never sole root-cause proof.",
	}
	type key struct{ action, ns string }
	stats := map[key]*OutcomeActionStat{}
	for _, r := range recs {
		sum.ByResult[r.Result]++
		sum.ByNamespace[r.Namespace]++
		k := key{action: r.Action, ns: r.Namespace}
		st, ok := stats[k]
		if !ok {
			st = &OutcomeActionStat{Action: r.Action, Namespace: r.Namespace}
			stats[k] = st
		}
		st.Total++
		switch r.Result {
		case "apply_success", "resolved":
			st.Success++
		case "apply_failed":
			st.Failed++
		case "apply_partial":
			st.Partial++
		}
	}
	for _, st := range stats {
		sum.ByAction = append(sum.ByAction, *st)
	}
	sort.Slice(sum.ByAction, func(i, j int) bool {
		if sum.ByAction[i].Total != sum.ByAction[j].Total {
			return sum.ByAction[i].Total > sum.ByAction[j].Total
		}
		if sum.ByAction[i].Action != sum.ByAction[j].Action {
			return sum.ByAction[i].Action < sum.ByAction[j].Action
		}
		return sum.ByAction[i].Namespace < sum.ByAction[j].Namespace
	})
	// Attach up to 10 newest records for context.
	n := len(recs)
	start := 0
	if n > 10 {
		start = n - 10
	}
	sum.Recent = append([]OutcomeRecord(nil), recs[start:]...)
	return sum
}

// LookupAction returns aggregated fleet stats for an action (RT-022 bias read).
// If namespace is non-empty, that namespace is preferred; otherwise (or when the
// namespace has no record) all namespaces for the action are summed. ok is false
// when the action has no fleet history at all.
func (s OutcomeSummary) LookupAction(action, namespace string) (stat OutcomeActionStat, ok bool) {
	action = strings.TrimSpace(action)
	namespace = strings.TrimSpace(namespace)
	if action == "" {
		return OutcomeActionStat{}, false
	}
	agg := OutcomeActionStat{Action: action, Namespace: namespace}
	for _, st := range s.ByAction {
		if st.Action != action {
			continue
		}
		if namespace != "" && st.Namespace == namespace {
			return st, true
		}
		agg.Success += st.Success
		agg.Failed += st.Failed
		agg.Partial += st.Partial
		agg.Total += st.Total
		ok = true
	}
	if !ok {
		return OutcomeActionStat{}, false
	}
	agg.Namespace = "" // aggregated across namespaces
	return agg, true
}

// FormatOutcomeSummary renders a compact human view (RT-021 CLI).
func FormatOutcomeSummary(s OutcomeSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Coordinator outcome ring · durable=%v · total=%d\n", s.Durable, s.Total)
	if len(s.ByResult) > 0 {
		results := make([]string, 0, len(s.ByResult))
		for k := range s.ByResult {
			results = append(results, k)
		}
		sort.Strings(results)
		b.WriteString("By result: ")
		parts := make([]string, 0, len(results))
		for _, r := range results {
			parts = append(parts, fmt.Sprintf("%s=%d", r, s.ByResult[r]))
		}
		b.WriteString(strings.Join(parts, " "))
		b.WriteByte('\n')
	}
	for i, st := range s.ByAction {
		if i >= 10 {
			break
		}
		fmt.Fprintf(&b, "- %s@%s success=%d failed=%d partial=%d (n=%d)\n",
			st.Action, st.Namespace, st.Success, st.Failed, st.Partial, st.Total)
	}
	if s.Note != "" {
		fmt.Fprintf(&b, "%s\n", s.Note)
	}
	return strings.TrimSpace(b.String())
}
