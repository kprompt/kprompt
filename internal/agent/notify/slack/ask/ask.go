// Package ask implements read-only Slack Q&A for the Observe agent (AG-019).
//
// Supported intents: status | why | what broke | false positive | approve. Approve mutates only via configured callback (RT-008).
package ask

import (
	"context"
	"fmt"
	"strings"

	"github.com/kprompt/kprompt/internal/agent/analyze"
	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/health"
	"github.com/kprompt/kprompt/internal/incident"
)

// Intent is a parsed ask command.
type Intent string

const (
	IntentStatus    Intent = "status"
	IntentWhy       Intent = "why"
	IntentWhatBroke Intent = "what_broke"
	IntentFalsePos  Intent = "false_positive"
	IntentApprove   Intent = "approve"
	IntentHelp      Intent = "help"
	IntentUnknown   Intent = "unknown"
)

// Handler answers Slack asks from open incidents + health (read-only for cluster mutate).
type Handler struct {
	OpenIncidents func() []incident.Incident
	Health        func(ctx context.Context) *health.Snapshot
	// Why builds a short RCA for an incident (optional — falls back to stored fields).
	Why func(ctx context.Context, inc incident.Incident) (analyze.Result, error)
	// MarkFalsePositive records AG-033 FP outcome (optional).
	MarkFalsePositive func(ctx context.Context, inc incident.Incident) error
	// ApproveProposal applies a durable AutopilotProposal by id (RT-008). Empty id → latest open incident proposal.
	// Must enforce policyAuto + allowlist + audit inside the callback; nil disables approve intent.
	ApproveProposal func(ctx context.Context, proposalID string) string
}

// ParseIntent maps free-form Slack text to a supported ask intent.
func ParseIntent(text string) Intent {
	t := strings.ToLower(strings.TrimSpace(text))
	fields := strings.Fields(t)
	cleaned := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.HasPrefix(f, "<@") && strings.HasSuffix(f, ">") {
			continue
		}
		cleaned = append(cleaned, f)
	}
	t = strings.Join(cleaned, " ")

	switch {
	case t == "" || t == "help" || strings.HasPrefix(t, "help "):
		return IntentHelp
	case strings.HasPrefix(t, "approve") || strings.HasPrefix(t, "apply "):
		return IntentApprove
	case strings.Contains(t, "false positive") || t == "fp" || strings.HasPrefix(t, "fp "):
		return IntentFalsePos
	case strings.Contains(t, "what broke") || strings.Contains(t, "whatbroke") || t == "broke":
		return IntentWhatBroke
	case strings.HasPrefix(t, "why") || strings.Contains(t, " root cause"):
		return IntentWhy
	case strings.HasPrefix(t, "status") || t == "health" || strings.HasPrefix(t, "how is"):
		return IntentStatus
	default:
		return IntentUnknown
	}
}

// Answer returns a plain-text Slack reply for the intent.
func (h *Handler) Answer(ctx context.Context, text string) string {
	if h == nil {
		return "ask handler is not configured"
	}
	switch ParseIntent(text) {
	case IntentHelp:
		return "Ask me: `status`, `why`, `what broke`, `false positive`, or `approve <proposal-id>`.\n" +
			"Approve = same gate as CLI `Apply this plan? [y/N]` / `--approve` (RemediationPolicy policyAuto + allowlist) — never silent apply.\n" +
			"Risk words match CLI: `low` · `medium` · `high` · `denied`."
	case IntentStatus:
		return h.answerStatus(ctx)
	case IntentWhatBroke:
		return h.answerWhatBroke()
	case IntentWhy:
		return h.answerWhy(ctx)
	case IntentFalsePos:
		return h.answerFalsePositive(ctx)
	case IntentApprove:
		return h.answerApprove(ctx, text)
	default:
		return "I only answer `status`, `why`, `what broke`, `false positive`, and `approve <proposal-id>` (read-only except approve). Try `help`.\n" +
			"`approve <id>` ≡ CLI `--approve` after you review the proposal."
	}
}

func (h *Handler) answerFalsePositive(ctx context.Context) string {
	incs := h.open()
	if len(incs) == 0 {
		return "No open incident to mark as false positive."
	}
	inc := incs[0]
	for _, cand := range incs {
		if severityRank(cand.Severity) > severityRank(inc.Severity) {
			inc = cand
		}
	}
	if h.MarkFalsePositive == nil {
		return "False-positive learning is not enabled (need --patterns)."
	}
	if err := h.MarkFalsePositive(ctx, inc); err != nil {
		return "Could not record false positive: " + err.Error()
	}
	return fmt.Sprintf("Recorded false positive for %s — future “seen before” boost will be dampened.", inc.ID)
}

func (h *Handler) answerApprove(ctx context.Context, text string) string {
	if h.ApproveProposal == nil {
		return "Proposal approve is not enabled (need --autopilot-propose + --slack-ask + policyAuto path)."
	}
	id := ParseApproveTarget(text)
	return h.ApproveProposal(ctx, id)
}

// ParseApproveTarget extracts an optional proposal id from Slack text (`approve` or `approve ap-…`).
func ParseApproveTarget(text string) string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(text)))
	cleaned := make([]string, 0, len(fields))
	for _, f := range fields {
		if strings.HasPrefix(f, "<@") && strings.HasSuffix(f, ">") {
			continue
		}
		cleaned = append(cleaned, f)
	}
	if len(cleaned) == 0 {
		return ""
	}
	if cleaned[0] != "approve" && cleaned[0] != "apply" {
		return ""
	}
	if len(cleaned) == 1 {
		return ""
	}
	if cleaned[1] == "proposal" && len(cleaned) >= 3 {
		return cleaned[2]
	}
	return cleaned[1]
}

func (h *Handler) answerStatus(ctx context.Context) string {
	var b strings.Builder
	if h.Health != nil {
		if snap := h.Health(ctx); snap != nil {
			fmt.Fprintf(&b, "Health %d/100 (%s) — open incidents: %d\n", snap.Score, snap.Trend, snap.OpenIncidents)
			if snap.Message != "" {
				fmt.Fprintf(&b, "%s\n", snap.Message)
			}
		}
	}
	incs := h.open()
	if len(incs) == 0 {
		b.WriteString("No open incidents in this namespace.")
		return b.String()
	}
	b.WriteString("Open:\n")
	for i, inc := range incs {
		if i >= 5 {
			fmt.Fprintf(&b, "…and %d more\n", len(incs)-5)
			break
		}
		fmt.Fprintf(&b, "• [%s] %s (conf=%.2f) id=%s\n",
			inc.Severity, firstNonEmpty(inc.Summary, "(no summary)"), inc.Confidence, inc.ID)
	}
	return strings.TrimSpace(b.String())
}

func (h *Handler) answerWhatBroke() string {
	incs := h.open()
	if len(incs) == 0 {
		return "Nothing open right now — no active incidents."
	}
	var b strings.Builder
	b.WriteString("What broke:\n")
	for i, inc := range incs {
		if i >= 5 {
			break
		}
		target := ""
		if inc.PrimaryResource != nil {
			target = inc.PrimaryResource.Kind + "/" + inc.PrimaryResource.Name
		}
		fmt.Fprintf(&b, "• %s — %s\n", firstNonEmpty(target, inc.ID), firstNonEmpty(inc.Summary, inc.RootCause))
	}
	return strings.TrimSpace(b.String())
}

func (h *Handler) answerWhy(ctx context.Context) string {
	incs := h.open()
	if len(incs) == 0 {
		return "No open incident to explain. Ask `status` first."
	}
	inc := incs[0] // newest-ish; OpenIncidents order is map iteration — still useful
	// Prefer highest severity
	for _, cand := range incs {
		if severityRank(cand.Severity) > severityRank(inc.Severity) {
			inc = cand
		}
	}
	if h.Why != nil {
		res, err := h.Why(ctx, inc)
		if err == nil && strings.TrimSpace(res.RootCause) != "" {
			return fmt.Sprintf("Why (%s): %s\nConfidence: %.2f\nRecommendation: %s",
				inc.ID, res.RootCause, res.Confidence, res.Recommendation)
		}
	}
	root := firstNonEmpty(inc.RootCause, "I don't have enough evidence yet")
	return fmt.Sprintf("Why (%s): %s\nConfidence: %.2f\nSummary: %s",
		inc.ID, root, inc.Confidence, firstNonEmpty(inc.Summary, "(none)"))
}

func (h *Handler) open() []incident.Incident {
	if h.OpenIncidents == nil {
		return nil
	}
	return h.OpenIncidents()
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func severityRank(s string) int {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case incident.SeverityCritical:
		return 5
	case incident.SeverityHigh:
		return 4
	case incident.SeverityMedium:
		return 3
	case incident.SeverityLow:
		return 2
	case incident.SeverityInfo:
		return 1
	default:
		return 0
	}
}

// WhyFromHeuristic builds a Why callback using ctxbuild + analyze Heuristic.
func WhyFromHeuristic(builder *ctxbuild.Builder) func(context.Context, incident.Incident) (analyze.Result, error) {
	return func(ctx context.Context, inc incident.Incident) (analyze.Result, error) {
		if builder == nil {
			return analyze.Result{}, fmt.Errorf("ctx builder is nil")
		}
		agentCtx := builder.Build(ctx, inc, ctxbuild.Options{})
		return analyze.Heuristic(agentCtx), nil
	}
}
