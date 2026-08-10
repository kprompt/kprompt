// Package coordinator is the thin cross-namespace fan-in service (AG-037…AG-039 / ADR-0017).
//
// It receives CoordinatorHandoff envelopes, optionally merges InvestigationReports,
// and never mutates workloads.
package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/kprompt/kprompt/internal/agent/handoff"
	"github.com/kprompt/kprompt/internal/incident"
)

const (
	APIVersion    = "kprompt.io/v1"
	KindReply     = "CoordinatorReply"
	SchemaVersion = "1"

	maxBody = 1 << 20 // 1 MiB

	// Probe EvidenceRef.Source values (AG-050 / AG-068).
	probeSourceKube  = "coordinator-kube-probe"
	probeSourceMerge = "coordinator-probe"

	// Confidence caps when cross-ns verify lacks independent anchors (AG-068).
	unverifiedConfidenceCap = 0.7 // no suspect report at all
	softAgreeConfidenceCap  = 0.4 // suspect present but narrative-only / soft-agree
)

// Reply is returned to the origin Namespace Agent after a handoff (AG-038).
type Reply struct {
	APIVersion       string                       `json:"apiVersion"`
	Kind             string                       `json:"kind"`
	SchemaVersion    string                       `json:"schemaVersion"`
	FromNamespace    string                       `json:"fromNamespace"`
	SuspectNamespace string                       `json:"suspectNamespace,omitempty"`
	Reason           string                       `json:"reason,omitempty"`
	CreatedAt        time.Time                    `json:"createdAt"`
	Merged           incident.InvestigationReport `json:"merged"`
	Routing          []string                     `json:"routing,omitempty"`
	// MutateAttempted is always false in this MVP (ADR-0017 §3).
	MutateAttempted bool `json:"mutateAttempted"`
}

// Record is one accepted handoff (in-memory; restart-lossy — fine for thin MVP).
type Record struct {
	Envelope handoff.Envelope `json:"envelope"`
	Reply    Reply            `json:"reply"`
	At       time.Time        `json:"at"`
}

// Service processes handoffs without cluster mutate.
type Service struct {
	mu      sync.Mutex
	recent  []Record
	maxKeep int
	// Probe is optional: fetch a suspect-ns InvestigationReport (AG-037 route).
	// Nil → routing note only; never invents foreign facts.
	Probe SuspectProber
	// Store is optional durable backend for the recent ring (AG-060).
	Store Store
	// PersistErrLog is optional; called when Save fails after a handoff.
	PersistErrLog func(error)
	// MaxHops caps blast-radius cascade depth from focus (RT-011). 0 → DefaultMaxHops.
	MaxHops int
	// MeshConfigured reports whether mesh/OTel enrichment is available (RT-010).
	// When false, /v1/blast-radius Status=degraded (honest).
	MeshConfigured bool

	// OutcomeStore is optional durable backend for the cross-ns outcome ring (RT-021).
	// If nil and Store implements OutcomeStore, that is used.
	OutcomeStore OutcomeStore
	// outcomes is the in-memory cross-ns outcome ring (RT-021).
	outcomes []OutcomeRecord
	// outcomeMaxKeep caps the outcome ring. 0 → DefaultOutcomeMaxKeep.
	outcomeMaxKeep int
	// outcomeTTL prunes stale outcomes. 0 → DefaultOutcomeTTL.
	outcomeTTL time.Duration

	audit []AuditEntry
}

// SuspectProber is a read-only verification hook for a suspect namespace.
type SuspectProber interface {
	Probe(ctx context.Context, suspectNamespace string, env handoff.Envelope) (*incident.InvestigationReport, error)
}

// NopProbe always returns no suspect report.
type NopProbe struct{}

func (NopProbe) Probe(context.Context, string, handoff.Envelope) (*incident.InvestigationReport, error) {
	return nil, nil
}

// New returns a Coordinator service with an in-memory ring of recent handoffs.
func New() *Service {
	return &Service{
		maxKeep:        100,
		Probe:          NopProbe{},
		outcomeMaxKeep: DefaultOutcomeMaxKeep,
		outcomeTTL:     DefaultOutcomeTTL,
	}
}

// SetOutcomeLimits overrides the outcome ring cap and TTL (RT-021).
// A non-positive value keeps the current default.
func (s *Service) SetOutcomeLimits(maxKeep int, ttl time.Duration) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if maxKeep > 0 {
		s.outcomeMaxKeep = maxKeep
	}
	if ttl > 0 {
		s.outcomeTTL = ttl
	}
	s.mu.Unlock()
}

// Handle processes one validated envelope → Reply (receive + merge + route notes).
func (s *Service) Handle(ctx context.Context, env handoff.Envelope) (Reply, error) {
	if err := handoff.Validate(env); err != nil {
		return Reply{}, err
	}
	var suspectRep *incident.InvestigationReport
	var routing []string
	suspectNS := strings.TrimSpace(env.SuspectNamespace)
	if suspectNS == "" {
		routing = append(routing, "no suspectNamespace on handoff — recorded for human / later probe")
	} else if s != nil && s.Probe != nil {
		rep, err := s.Probe.Probe(ctx, suspectNS, env)
		if err != nil {
			routing = append(routing, fmt.Sprintf("probe %s failed: %v", suspectNS, err))
		} else if rep != nil {
			suspectRep = rep
			routing = append(routing, fmt.Sprintf("probed namespace %s — merged suspect InvestigationReport", suspectNS))
		} else {
			routing = append(routing, fmt.Sprintf("suspect namespace %s not probed (no probe result) — unknowns retained", suspectNS))
		}
	} else {
		routing = append(routing, fmt.Sprintf("suspect namespace %s — probe not configured", suspectNS))
	}
	routing = append(routing, "mutateAttempted=false (ADR-0017)")

	merged := Merge(env.Report, suspectRep, suspectNS)
	reply := Reply{
		APIVersion:       APIVersion,
		Kind:             KindReply,
		SchemaVersion:    SchemaVersion,
		FromNamespace:    env.FromNamespace,
		SuspectNamespace: suspectNS,
		Reason:           env.Reason,
		CreatedAt:        time.Now().UTC(),
		Merged:           merged,
		Routing:          routing,
		MutateAttempted:  false,
	}

	if s != nil {
		s.mu.Lock()
		s.recent = append(s.recent, Record{Envelope: env, Reply: reply, At: reply.CreatedAt})
		if s.maxKeep > 0 && len(s.recent) > s.maxKeep {
			s.recent = s.recent[len(s.recent)-s.maxKeep:]
		}
		snap := Snapshot{SchemaVersion: SchemaVersion, Records: append([]Record(nil), s.recent...)}
		store := s.Store
		logf := s.PersistErrLog
		kind := "handoff"
		if env.Reason == reasonProactiveTick {
			kind = reasonProactiveTick
		}
		s.audit = append(s.audit, AuditEntry{
			At: reply.CreatedAt, Kind: kind,
			From: env.FromNamespace, Suspect: suspectNS,
			MutateAttempted: false,
			Detail:           strings.Join(routing, "; "),
		})
		if len(s.audit) > maxAuditKeep {
			s.audit = s.audit[len(s.audit)-maxAuditKeep:]
		}
		s.mu.Unlock()
		if store != nil {
			if err := store.Save(snap); err != nil && logf != nil {
				logf(err)
			}
		}
	}
	return reply, nil
}

// Restore loads durable Shared Knowledge into the recent ring (AG-060).
// Safe to call before ListenAndServe. Truncates to maxKeep.
func (s *Service) Restore() error {
	if s == nil || s.Store == nil {
		return nil
	}
	snap, err := s.Store.Load()
	if err != nil {
		return err
	}
	recs := snap.Records
	if s.maxKeep > 0 && len(recs) > s.maxKeep {
		recs = recs[len(recs)-s.maxKeep:]
	}
	s.mu.Lock()
	s.recent = append([]Record(nil), recs...)
	s.mu.Unlock()
	return s.RestoreOutcomes()
}

// Durable reports whether a Store is configured (AG-060).
func (s *Service) Durable() bool {
	return s != nil && s.Store != nil
}

// Knowledge returns the Shared Knowledge summary for the current ring.
func (s *Service) Knowledge() KnowledgeSummary {
	return Summarize(s.Recent(), s.Durable())
}

// Recent returns a copy of recent handoff records (newest last).
func (s *Service) Recent() []Record {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Record, len(s.recent))
	copy(out, s.recent)
	return out
}

// Merge combines origin + optional suspect InvestigationReports (AG-038 / AG-068).
// Never invents cluster facts. Independent verify requires fresh probe EvidenceRef
// or honest probe Unknowns — a high-confidence narrative soft-agree does not count.
func Merge(origin incident.InvestigationReport, suspect *incident.InvestigationReport, suspectNS string) incident.InvestigationReport {
	out := origin
	out.Kind = incident.KindInvestigationReport
	if out.SchemaVersion == "" {
		out.SchemaVersion = incident.SchemaVersion2
	}
	if out.APIVersion == "" {
		out.APIVersion = incident.APIVersion
	}
	out.CreatedAt = time.Now().UTC()

	suspectNS = strings.TrimSpace(suspectNS)
	if suspect == nil {
		note := "Coordinator: cross-namespace verification pending"
		if suspectNS != "" {
			note = fmt.Sprintf("Coordinator: namespace %q not verified yet — do not treat origin hypotheses as foreign-ns facts", suspectNS)
		}
		out.Unknowns = appendUnique(out.Unknowns, note)
		if out.Confidence > unverifiedConfidenceCap {
			out.Confidence = unverifiedConfidenceCap
		}
		return out
	}

	// AG-068: narrative / same-session soft-agree is not a verify edge.
	if !hasFreshProbeEvidence(suspect) && !hasHonestProbeUnknowns(suspect) {
		nsLabel := firstNonEmpty(suspect.Namespace, suspectNS)
		if suspect.Summary != "" {
			out.Summary = strings.TrimSpace(out.Summary + "; suspect@" + nsLabel + ": " + suspect.Summary)
		}
		for _, u := range suspect.Unknowns {
			out.Unknowns = appendUnique(out.Unknowns, u)
		}
		note := "Coordinator: independent verify failed — suspect report lacks probe EvidenceRef or honest probe Unknowns (narrative soft-agree is not verification)"
		if nsLabel != "" {
			note = fmt.Sprintf("Coordinator: independent verify failed for %q — suspect report lacks probe EvidenceRef or honest probe Unknowns (narrative soft-agree is not verification)", nsLabel)
		}
		out.Unknowns = appendUnique(out.Unknowns, note)
		if out.Confidence > softAgreeConfidenceCap {
			out.Confidence = softAgreeConfidenceCap
		}
		out.Reasoning = strings.TrimSpace(out.Reasoning + "; coordinator-merge-unverified")
		return out
	}

	// Tag and append suspect evidence.
	for _, e := range suspect.Evidence {
		e.Source = firstNonEmpty(e.Source, probeSourceMerge)
		if e.Resource != nil && e.Resource.Namespace == "" && suspect.Namespace != "" {
			cp := *e.Resource
			cp.Namespace = suspect.Namespace
			e.Resource = &cp
		}
		out.Evidence = append(out.Evidence, e)
	}
	for _, e := range suspect.Timeline {
		out.Timeline = append(out.Timeline, e)
	}
	for _, h := range suspect.Hypotheses {
		h.Statement = strings.TrimSpace(h.Statement)
		if h.Statement == "" {
			continue
		}
		if suspect.Namespace != "" && !strings.Contains(h.Statement, suspect.Namespace) {
			h.Statement = fmt.Sprintf("[%s] %s", suspect.Namespace, h.Statement)
		}
		h.Primary = false
		out.Hypotheses = append(out.Hypotheses, h)
	}
	for _, u := range suspect.Unknowns {
		out.Unknowns = appendUnique(out.Unknowns, u)
	}
	for _, d := range suspect.Degraded {
		out.Degraded = appendUnique(out.Degraded, d)
	}
	if suspect.Summary != "" {
		out.Summary = strings.TrimSpace(out.Summary + "; suspect@" + firstNonEmpty(suspect.Namespace, suspectNS) + ": " + suspect.Summary)
	}
	// Probe anchors are the confidence ceiling — never keep origin LLM score above probe.
	if suspect.Confidence > 0 && suspect.Confidence < out.Confidence {
		out.Confidence = suspect.Confidence
	}
	if out.Confidence > 0.05 {
		out.Confidence = clamp01(out.Confidence - 0.05)
	}
	out.Unknowns = appendUnique(out.Unknowns, "Coordinator: merged origin + suspect reports (verify before mutate)")
	out.Reasoning = strings.TrimSpace(out.Reasoning + "; coordinator-merge")
	return out
}

func isProbeSource(src string) bool {
	switch strings.ToLower(strings.TrimSpace(src)) {
	case probeSourceKube, probeSourceMerge:
		return true
	default:
		return false
	}
}

// hasFreshProbeEvidence reports whether the suspect carries cluster EvidenceRef
// stamped by KubeProbe (or equivalently sourced for merge).
func hasFreshProbeEvidence(suspect *incident.InvestigationReport) bool {
	if suspect == nil {
		return false
	}
	probeReasoning := strings.EqualFold(strings.TrimSpace(suspect.Reasoning), probeSourceKube)
	for _, e := range suspect.Evidence {
		if isProbeSource(e.Source) {
			return true
		}
		// KubeProbe always sets Reasoning; accept typed cluster reads even if Source was omitted.
		if probeReasoning {
			switch e.Type {
			case incident.EvidenceEvent, incident.EvidenceObject, incident.EvidenceMetric:
				return true
			}
		}
	}
	return false
}

// hasHonestProbeUnknowns accepts soft-fail probe reports that could not gather
// Evidence but recorded Unknowns (e.g. RBAC / empty list) under kube-probe reasoning.
func hasHonestProbeUnknowns(suspect *incident.InvestigationReport) bool {
	if suspect == nil || len(suspect.Unknowns) == 0 {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(suspect.Reasoning), probeSourceKube)
}

// Handler serves HTTP endpoints for the Coordinator (AG-037).
type Handler struct {
	Service *Service
	Logf    func(string, ...any)
}

func (h *Handler) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", h.healthz)
	mux.HandleFunc("/v1/handoff", h.handoff)
	mux.HandleFunc("/v1/recent", h.recent)
	mux.HandleFunc("/v1/knowledge", h.knowledge)     // AG-059 Shared Knowledge MVP
	mux.HandleFunc("/v1/blast-radius", h.blastRadius) // AG-066 blast-radius product graph MVP
	mux.HandleFunc("/v1/outcome", h.outcome)          // RT-021 record cross-ns outcome
	mux.HandleFunc("/v1/outcomes", h.outcomes)        // RT-021/RT-022 outcome ring summary
	return mux
}

func (h *Handler) outcome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var rec OutcomeRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if err := h.Service.RecordOutcome(rec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.Logf != nil {
		h.Logf("outcome ns=%s action=%q result=%s mutate=false", rec.Namespace, rec.Action, rec.Result)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "recorded"})
}

func (h *Handler) outcomes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.Service.OutcomeSummarize())
}

// ListenAndServe runs until ctx is cancelled.
// Optional tickCfg.Interval > 0 starts the proactive correlation ticker (RT-009).
func ListenAndServe(ctx context.Context, addr string, h *Handler, tickCfg ...TickConfig) error {
	if h == nil {
		h = &Handler{Service: New()}
	}
	if h.Service == nil {
		h.Service = New()
	}
	var cfg TickConfig
	if len(tickCfg) > 0 {
		cfg = tickCfg[0]
		if cfg.MaxHops > 0 {
			h.Service.MaxHops = cfg.MaxHops
		}
	}
	if cfg.Interval > 0 {
		go RunTicker(ctx, h.Service, cfg, h.Logf)
	}
	srv := &http.Server{Addr: addr, Handler: h.routes()}
	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
		return ctx.Err()
	case err := <-errCh:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func (h *Handler) healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok\n"))
}

func (h *Handler) handoff(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "POST only", http.StatusMethodNotAllowed)
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, maxBody))
	if err != nil {
		http.Error(w, "read body", http.StatusBadRequest)
		return
	}
	var env handoff.Envelope
	if err := json.Unmarshal(body, &env); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}
	if env.APIVersion == "" {
		env.APIVersion = handoff.APIVersion
	}
	if env.SchemaVersion == "" {
		env.SchemaVersion = handoff.SchemaVersion
	}
	reply, err := h.Service.Handle(r.Context(), env)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if h.Logf != nil {
		h.Logf("handoff from=%s suspect=%s reason=%q mutate=false",
			reply.FromNamespace, reply.SuspectNamespace, reply.Reason)
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(reply)
}

func (h *Handler) recent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.Service.Recent())
}

func appendUnique(list []string, v string) []string {
	v = strings.TrimSpace(v)
	if v == "" {
		return list
	}
	for _, x := range list {
		if x == v {
			return list
		}
	}
	return append(list, v)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
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
