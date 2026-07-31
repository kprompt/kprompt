// Package analyze turns AgentContext into a gated AgentAlert (AG-008).
//
// Pipeline: context → (optional LLM structured completion) → confidence/severity gate → AgentAlert.
// One LLM call per incident evidence fingerprint — not per raw watch event.
package analyze

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/kprompt/kprompt/internal/agent/confidence"
	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/detect"
	"github.com/kprompt/kprompt/internal/agent/handoff"
	"github.com/kprompt/kprompt/internal/agent/patterns"
	"github.com/kprompt/kprompt/internal/agent/priority"
	"github.com/kprompt/kprompt/internal/incident"
	"github.com/kprompt/kprompt/internal/llm"
)

// Result from a structured LLM (or heuristic) analysis.
type Result struct {
	Severity       string  `json:"severity"`
	Confidence     float64 `json:"confidence"`
	Summary        string  `json:"summary"`
	RootCause      string  `json:"rootCause"`
	Recommendation string  `json:"recommendation"`
	// DetectorCode is set when the heuristic catalog matched (AG-026).
	DetectorCode string `json:"detectorCode,omitempty"`
	// CausalChain is symptom → … → probable root (AG-028 / ADR-0016).
	CausalChain []string `json:"causalChain,omitempty"`
	// Alternatives are non-primary hypotheses.
	Alternatives []string `json:"alternatives,omitempty"`
	// Objective is ADR-0016 §5 priority class (AG-030).
	Objective string `json:"objective,omitempty"`
}

var analysisSchema = json.RawMessage(`{
  "type": "object",
  "required": ["severity", "confidence", "summary", "rootCause", "recommendation"],
  "properties": {
    "severity": {"type": "string", "enum": ["info", "low", "medium", "high", "critical"]},
    "confidence": {"type": "number", "minimum": 0, "maximum": 1},
    "summary": {"type": "string"},
    "rootCause": {"type": "string"},
    "recommendation": {"type": "string"}
  }
}`)

const systemPrompt = `You are kprompt Observe Mode, an AI SRE assistant for Kubernetes.
Given structured incident context, produce a concise root-cause analysis.
Rules:
- Use only evidence in the context; do not invent cluster facts.
- namespace_memory facts are priors/hints only — never sole proof of root cause; require Events/logs/metrics/traces/gitops.
- If signals are weak, lower confidence and say you do not have enough evidence.
- recommendation must be safe guidance (check/verify); never claim you mutated the cluster.
- Prefer severity that reflects impact priority: outage/data-loss/security before performance/cost/hygiene.
- severity must be one of: info, low, medium, high, critical.
- Respond with JSON only matching the schema.`

// Options configures the analyzer gate.
type Options struct {
	MinSeverity   string
	MinConfidence float64
	// HeuristicOnly skips the LLM even when Provider is set (tests).
	HeuristicOnly bool
}

// Analyzer maps AgentContext → AgentAlert with dedupe + gate.
type Analyzer struct {
	Provider llm.Provider
	Options  Options
	// Patterns optional AG-016 library — boosts confidence on “seen before”; never mutates.
	Patterns *patterns.Library

	mu       sync.Mutex
	lastFP   map[string]string // incidentID → evidence fingerprint
	lastPass map[string]bool   // last gate result (for updated suppression noise)
}

// New returns an analyzer with Observe defaults.
func New(provider llm.Provider, opts Options) *Analyzer {
	if opts.MinSeverity == "" {
		opts.MinSeverity = incident.DefaultMinSeverity()
	}
	if opts.MinConfidence <= 0 {
		opts.MinConfidence = incident.DefaultMinConfidence()
	}
	return &Analyzer{
		Provider: provider,
		Options:  opts,
		lastFP:   map[string]string{},
		lastPass: map[string]bool{},
	}
}

// AnalyzeOutcome is returned to the agent loop.
type AnalyzeOutcome struct {
	Alert       incident.AgentAlert `json:"alert"`
	PassedGate  bool                `json:"passedGate"`
	Skipped     bool                `json:"skipped,omitempty"` // deduped LLM call
	Source      string              `json:"source"`            // llm | heuristic
	Result      Result              `json:"result"`
	SeenBefore  string              `json:"seenBefore,omitempty"` // AG-016 note
	PatternHits int                 `json:"patternHits,omitempty"`
	// Report is InvestigationReport v2 (AG-022 / AG-028 / AG-031). Optional for Observe V1 consumers.
	Report incident.InvestigationReport `json:"report,omitempty"`
}

// Analyze runs structured analysis for an open/updated incident context.
// alertStatus should be fired|updated|recovered.
func (a *Analyzer) Analyze(ctx context.Context, agentCtx ctxbuild.AgentContext, alertStatus string) (AnalyzeOutcome, error) {
	if a == nil {
		return AnalyzeOutcome{}, fmt.Errorf("analyze: analyzer is nil")
	}
	inc := agentCtx.Incident
	fp := fingerprint(inc)
	a.mu.Lock()
	prev, seen := a.lastFP[inc.ID]
	if seen && prev == fp {
		a.mu.Unlock()
		return AnalyzeOutcome{Skipped: true, Source: "dedupe"}, nil
	}
	a.lastFP[inc.ID] = fp
	a.mu.Unlock()

	var (
		res    Result
		source string
		err    error
	)
	useLLM := a.Provider != nil && !a.Options.HeuristicOnly
	if useLLM {
		res, err = a.callLLM(ctx, agentCtx)
		source = "llm"
		if err != nil {
			// Fall back to heuristic rather than failing the watch loop.
			res = Heuristic(agentCtx)
			source = "heuristic"
			err = nil
		}
	} else {
		res = Heuristic(agentCtx)
		source = "heuristic"
	}

	normalizeResult(&res, inc)
	applyPriority(&res, agentCtx)

	matched := strings.TrimSpace(res.DetectorCode) != ""
	llmTrusted := source == "llm"
	if adj, note := confidence.Adjust(res.Confidence, agentCtx, matched, llmTrusted); adj != res.Confidence || note != "" {
		res.Confidence = adj
		if !llmTrusted && !matched && (note == "not enough evidence" || note == "weak evidence") {
			res.RootCause = confidence.NotEnoughEvidenceRoot
			if len(res.Alternatives) == 0 {
				res.Alternatives = []string{"Need more Events/logs/metrics before asserting root cause"}
			}
		}
	}

	recordRoot := res.RootCause
	recordRec := res.Recommendation

	var seenNote string
	var hits int
	if a.Patterns != nil {
		if match, ok := a.Patterns.Match(agentCtx.Namespace, agentCtx); ok {
			hits = match.Count
			boosted, note := patterns.ApplyBoost(patterns.SeverityConfidence{
				Confidence:     res.Confidence,
				RootCause:      res.RootCause,
				Recommendation: res.Recommendation,
			}, match)
			res.Confidence = boosted.Confidence
			res.RootCause = boosted.RootCause
			res.Recommendation = boosted.Recommendation
			seenNote = note
		}
	}

	// Enrich incident fields for NewAgentAlert
	inc.Severity = res.Severity
	inc.Confidence = res.Confidence
	inc.Summary = res.Summary
	inc.RootCause = res.RootCause
	inc.Recommendation = res.Recommendation

	if alertStatus == "" {
		alertStatus = incident.AlertFired
	}
	alert := incident.NewAgentAlert(inc, alertStatus, time.Now().UTC())
	alert.Affected = append([]incident.ResourceRef(nil), inc.Affected...)
	if len(alert.Evidence) == 0 {
		alert.Evidence = append([]incident.EvidenceRef(nil), agentCtx.LogSnippets...)
		alert.Evidence = append(alert.Evidence, agentCtx.RecentEvents...)
	}

	passed := incident.MeetsAlertGate(alert, a.Options.MinSeverity, a.Options.MinConfidence)
	if err := incident.ValidateAgentAlert(alert); err != nil && passed {
		passed = false
	}

	// Learn after analysis (Observe-only — never applies a mutate from the pattern).
	if a.Patterns != nil {
		if alertStatus == incident.AlertRecovered {
			if _, rerr := a.Patterns.RecordOutcome(agentCtx.Namespace, agentCtx, patterns.OutcomeResolved); rerr != nil {
				// Non-fatal: no prior pattern yet is fine.
			}
		} else {
			if _, rerr := a.Patterns.Record(agentCtx.Namespace, agentCtx, res.Severity, res.Summary, recordRoot, recordRec); rerr != nil {
				// Non-fatal: learning must not break the watch loop.
			}
		}
	}

	a.mu.Lock()
	a.lastPass[inc.ID] = passed
	a.mu.Unlock()

	report := buildReport(inc, agentCtx, res, seenNote)

	return AnalyzeOutcome{
		Alert:       alert,
		PassedGate:  passed,
		Source:      source,
		Result:      res,
		SeenBefore:  seenNote,
		PatternHits: hits,
		Report:      report,
	}, nil
}

func (a *Analyzer) callLLM(ctx context.Context, agentCtx ctxbuild.AgentContext) (Result, error) {
	user := strings.Join(agentCtx.PromptBlocks(), "\n")
	raw, err := a.Provider.CompleteStructured(ctx, llm.CompletionRequest{
		System: systemPrompt,
		User:   user,
	}, analysisSchema)
	if err != nil {
		return Result{}, err
	}
	var res Result
	if err := json.Unmarshal(raw, &res); err != nil {
		return Result{}, fmt.Errorf("analyze: decode LLM JSON: %w", err)
	}
	return res, nil
}

// Heuristic builds a Result without an LLM (offline / fallback).
// Uses the AG-026 detector catalog for causal-chain RCA when a detector matches.
func Heuristic(agentCtx ctxbuild.AgentContext) Result {
	inc := agentCtx.Incident
	sev := inc.Severity
	if sev == "" {
		sev = incident.SeverityMedium
	}
	summary := inc.Summary
	if summary == "" {
		summary = "Problem signal detected"
	}
	root := "Insufficient automated signal for a precise root cause"
	rec := "Inspect pod events and recent logs; verify dependent Services/Endpoints"
	conf := 0.55
	var chain []string
	var alts []string
	var code string

	if hit, ok := detect.Catalog(agentCtx); ok {
		code = hit.Code
		sev = hit.Severity
		conf = hit.Confidence
		if strings.TrimSpace(hit.Summary) != "" {
			summary = hit.Summary
		}
		root = hit.RootCause
		rec = hit.Recommendation
		chain = append([]string(nil), hit.CausalChain...)
		alts = append([]string(nil), hit.Alternatives...)
	}

	if agentCtx.Deployment != nil && agentCtx.Deployment.ChangeCause != "" {
		root = root + "; recent change-cause: " + agentCtx.Deployment.ChangeCause
		conf = minFloat(conf+0.05, 0.95)
		if len(chain) > 0 {
			chain = append(chain, "Recent deployment change-cause recorded")
		}
	}
	for _, g := range agentCtx.GitOps {
		if g.Reason == "out_of_sync" || g.Reason == "unhealthy" {
			conf = minFloat(conf+0.03, 0.95)
			if !strings.Contains(strings.ToLower(root), "gitops") {
				root = root + "; GitOps " + g.Reason
			}
			break
		}
	}
	if len(agentCtx.Degraded) > 0 {
		conf = maxFloat(conf-0.1, 0.3)
	}
	res := Result{
		Severity:       sev,
		Confidence:     conf,
		Summary:        summary,
		RootCause:      root,
		Recommendation: rec,
		DetectorCode:   code,
		CausalChain:    chain,
		Alternatives:   alts,
	}
	applyPriority(&res, agentCtx)
	return res
}

// applyPriority stamps ADR-0016 objective and raises severity to the objective floor (AG-030).
func applyPriority(res *Result, agentCtx ctxbuild.AgentContext) {
	if res == nil {
		return
	}
	c := priority.Classify(agentCtx, res.DetectorCode, res.Severity, res.Summary, res.RootCause)
	res.Objective = c.Objective
	res.Severity = priority.ApplySeverity(res.Severity, c.Objective)
}

func buildReport(inc incident.Incident, agentCtx ctxbuild.AgentContext, res Result, seenNote string) incident.InvestigationReport {
	rep := incident.ReportFromIncident(inc, time.Now().UTC())
	rep.Summary = res.Summary
	rep.Confidence = res.Confidence
	rep.Severity = res.Severity
	rep.Facts = buildFacts(inc, agentCtx)
	rep.Reasoning = buildReasoning(res, seenNote)
	rep.Unknowns = append([]string(nil), agentCtx.Degraded...)
	for _, d := range agentCtx.Degraded {
		rep.Degraded = append(rep.Degraded, d)
	}
	if len(rep.Evidence) == 0 {
		rep.Evidence = append([]incident.EvidenceRef(nil), agentCtx.RecentEvents...)
		rep.Evidence = append(rep.Evidence, agentCtx.LogSnippets...)
	}
	rep.Evidence = append(rep.Evidence, agentCtx.Metrics...)
	rep.Evidence = append(rep.Evidence, agentCtx.Traces...)
	rep.Evidence = append(rep.Evidence, agentCtx.GitOps...)
	rep.Timeline = append([]incident.EvidenceRef(nil), rep.Evidence...)

	hyps := make([]incident.Hypothesis, 0, 1+len(res.Alternatives))
	hyps = append(hyps, incident.Hypothesis{
		Statement:   res.RootCause,
		CausalChain: append([]string(nil), res.CausalChain...),
		Confidence:  res.Confidence,
		Primary:     true,
	})
	for _, alt := range res.Alternatives {
		alt = strings.TrimSpace(alt)
		if alt == "" {
			continue
		}
		hyps = append(hyps, incident.Hypothesis{
			Statement:  alt,
			Confidence: maxFloat(res.Confidence-0.4, 0.15),
		})
	}
	rep.Hypotheses = hyps
	rep.RecommendedActions = []incident.RecommendedAction{{
		Title:      res.Recommendation,
		Why:        res.RootCause,
		Confidence: res.Confidence,
		ActionID:   res.Objective,
	}}
	priority.SortActions(rep.RecommendedActions)
	if res.Objective != "" {
		rep.Reasoning = firstNonEmptyJoin(rep.Reasoning, "objective="+res.Objective)
	}
	if seenNote != "" {
		rep.Unknowns = append(rep.Unknowns, seenNote)
	}
	annotateCrossNamespace(&rep, agentCtx)
	return rep
}

// annotateCrossNamespace stamps honest Unknowns when evidence/memory mention a foreign ns (AG-053).
func annotateCrossNamespace(rep *incident.InvestigationReport, agentCtx ctxbuild.AgentContext) {
	if rep == nil {
		return
	}
	fromNS := strings.TrimSpace(rep.Namespace)
	if fromNS == "" {
		fromNS = strings.TrimSpace(agentCtx.Namespace)
	}
	var parts []string
	for _, e := range rep.Evidence {
		parts = append(parts, e.Message, e.Reason)
	}
	for _, e := range agentCtx.LogSnippets {
		parts = append(parts, e.Message, e.Reason)
	}
	for _, f := range agentCtx.Memory {
		parts = append(parts, f.Key, f.Value, f.Evidence)
	}
	blob := strings.Join(parts, " ")
	suspect, reason, ok := handoff.NeedsHandoff(fromNS, incident.InvestigationReport{
		Namespace: fromNS,
		Summary:   blob,
		Unknowns:  nil,
	})
	if !ok {
		return
	}
	note := reason
	if suspect != "" {
		note = fmt.Sprintf("cross-namespace dependency may involve namespace %q — need Coordinator verification", suspect)
	}
	for _, u := range rep.Unknowns {
		if u == note {
			return
		}
	}
	rep.Unknowns = append(rep.Unknowns, note)
}

func firstNonEmptyJoin(a, b string) string {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	switch {
	case a == "":
		return b
	case b == "":
		return a
	default:
		return a + "; " + b
	}
}

func buildFacts(inc incident.Incident, agentCtx ctxbuild.AgentContext) string {
	var parts []string
	if s := strings.TrimSpace(inc.Summary); s != "" {
		parts = append(parts, s)
	}
	if agentCtx.Pod != nil {
		parts = append(parts, "pod phase="+agentCtx.Pod.Phase)
	}
	if agentCtx.Deployment != nil && agentCtx.Deployment.ChangeCause != "" {
		parts = append(parts, "change-cause="+agentCtx.Deployment.ChangeCause)
	}
	nEv := len(inc.Evidence) + len(agentCtx.RecentEvents) + len(agentCtx.LogSnippets)
	if nEv > 0 {
		parts = append(parts, fmt.Sprintf("evidence refs=%d", nEv))
	}
	return strings.Join(parts, "; ")
}

func buildReasoning(res Result, seenNote string) string {
	var parts []string
	if code := strings.TrimSpace(res.DetectorCode); code != "" {
		parts = append(parts, "detector="+code)
	}
	if len(res.CausalChain) > 0 {
		parts = append(parts, "chain="+strings.Join(res.CausalChain, " → "))
	}
	if seenNote != "" {
		parts = append(parts, seenNote)
	}
	if len(parts) == 0 {
		return "Analysis based on available incident evidence only."
	}
	return strings.Join(parts, "; ")
}

func normalizeResult(res *Result, inc incident.Incident) {
	res.Severity = strings.ToLower(strings.TrimSpace(res.Severity))
	switch res.Severity {
	case incident.SeverityInfo, incident.SeverityLow, incident.SeverityMedium,
		incident.SeverityHigh, incident.SeverityCritical:
	default:
		if inc.Severity != "" {
			res.Severity = inc.Severity
		} else {
			res.Severity = incident.SeverityMedium
		}
	}
	if res.Confidence < 0 {
		res.Confidence = 0
	}
	if res.Confidence > 1 {
		res.Confidence = 1
	}
	if strings.TrimSpace(res.Summary) == "" {
		res.Summary = firstNonEmpty(inc.Summary, "Incident "+inc.ID)
	}
	if strings.TrimSpace(res.RootCause) == "" {
		res.RootCause = "Unknown"
	}
	if strings.TrimSpace(res.Recommendation) == "" {
		res.Recommendation = "Investigate with kprompt \"investigate <workload>\" or explain / kubectl describe"
	}
}

func fingerprint(inc incident.Incident) string {
	h := sha256.New()
	_, _ = fmt.Fprintf(h, "%s|%s|%d|", inc.ID, inc.Status, len(inc.Evidence))
	for _, e := range inc.Evidence {
		_, _ = fmt.Fprintf(h, "%s|%s|%s|", e.Type, e.Reason, e.Message)
	}
	return hex.EncodeToString(h.Sum(nil))
}

func joinEvidence(ev []incident.EvidenceRef) string {
	var b strings.Builder
	for _, e := range ev {
		b.WriteString(e.Reason)
		b.WriteByte(' ')
		b.WriteString(e.Message)
		b.WriteByte(' ')
	}
	return b.String()
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

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// AlertStatusFor maps correlate change kinds to AgentAlert status.
func AlertStatusFor(changeKind string) string {
	switch changeKind {
	case "closed":
		return incident.AlertRecovered
	case "updated", "reopened":
		return incident.AlertUpdated
	default:
		return incident.AlertFired
	}
}
