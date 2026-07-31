// Package handoff sends CoordinatorHandoff envelopes from a Namespace Agent (AG-036 / ADR-0017).
//
// Ns agents never invent foreign-namespace facts; they ask the Coordinator to verify.
package handoff

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/incident"
)

const (
	APIVersion    = "kprompt.io/v1"
	Kind          = "CoordinatorHandoff"
	KindReply     = "CoordinatorReply"
	SchemaVersion = "1"
)

// Envelope is the ns → Coordinator handoff document.
type Envelope struct {
	APIVersion       string                       `json:"apiVersion"`
	Kind             string                       `json:"kind"`
	SchemaVersion    string                       `json:"schemaVersion"`
	FromNamespace    string                       `json:"fromNamespace"`
	SuspectNamespace string                       `json:"suspectNamespace,omitempty"`
	Reason           string                       `json:"reason"`
	Urgency          string                       `json:"urgency,omitempty"`
	CreatedAt        time.Time                    `json:"createdAt"`
	Report           incident.InvestigationReport `json:"report"`
}

// Reply is the Coordinator → origin agent response (AG-049; mirrors CoordinatorReply JSON).
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
	MutateAttempted  bool                         `json:"mutateAttempted"`
}

// New builds a schema-stamped handoff around an InvestigationReport v2.
func New(fromNS, suspectNS, reason string, report incident.InvestigationReport) Envelope {
	return Envelope{
		APIVersion:       APIVersion,
		Kind:             Kind,
		SchemaVersion:    SchemaVersion,
		FromNamespace:    strings.TrimSpace(fromNS),
		SuspectNamespace: strings.TrimSpace(suspectNS),
		Reason:           strings.TrimSpace(reason),
		CreatedAt:        time.Now().UTC(),
		Report:           report,
	}
}

// Validate checks envelope + embedded report.
func Validate(e Envelope) error {
	if e.Kind != Kind {
		return fmt.Errorf("handoff: kind must be %s", Kind)
	}
	if strings.TrimSpace(e.FromNamespace) == "" {
		return fmt.Errorf("handoff: fromNamespace is required")
	}
	if strings.TrimSpace(e.Reason) == "" {
		return fmt.Errorf("handoff: reason is required")
	}
	if err := incident.ValidateInvestigationReport(e.Report); err != nil {
		return fmt.Errorf("handoff: report: %w", err)
	}
	return nil
}

// Client delivers handoffs and returns the Coordinator reply when available (AG-049).
type Client interface {
	Handoff(ctx context.Context, env Envelope) (*Reply, error)
}

// NopClient discards handoffs (tests / disabled).
type NopClient struct{}

func (NopClient) Handoff(context.Context, Envelope) (*Reply, error) { return nil, nil }

// HTTPClient POSTs JSON envelopes to a Coordinator URL.
type HTTPClient struct {
	URL        string
	HTTPClient *http.Client
}

func (c *HTTPClient) Handoff(ctx context.Context, env Envelope) (*Reply, error) {
	if c == nil || strings.TrimSpace(c.URL) == "" {
		return nil, fmt.Errorf("handoff: URL is required")
	}
	if err := Validate(env); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(env)
	if err != nil {
		return nil, err
	}
	hc := c.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.URL, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("handoff: read reply: %w", err)
	}
	if res.StatusCode >= 300 {
		return nil, fmt.Errorf("handoff: HTTP %d", res.StatusCode)
	}
	var reply Reply
	if len(bytes.TrimSpace(body)) == 0 {
		return &Reply{Kind: KindReply}, nil
	}
	if err := json.Unmarshal(body, &reply); err != nil {
		return nil, fmt.Errorf("handoff: decode reply: %w", err)
	}
	if reply.Kind == "" {
		reply.Kind = KindReply
	}
	return &reply, nil
}

var (
	reNamespacePhrase = regexp.MustCompile(`(?i)\b(?:namespace|ns)[/:\s]+([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)`)
	reSvcDNS          = regexp.MustCompile(`(?i)\b[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?\.([a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)\.svc(?:\.cluster\.local)?\b`)
)

// NeedsHandoff is a cheap heuristic: report Unknowns or root/summary mention another namespace (AG-048).
func NeedsHandoff(fromNS string, report incident.InvestigationReport) (suspect string, reason string, ok bool) {
	fromNS = strings.TrimSpace(fromNS)
	parts := []string{report.Summary, report.RootCauseHint(), report.Facts, report.Reasoning}
	parts = append(parts, report.Unknowns...)
	for _, h := range report.Hypotheses {
		parts = append(parts, h.Statement)
		parts = append(parts, h.CausalChain...)
	}
	for _, e := range report.Evidence {
		parts = append(parts, e.Message, e.Reason, e.URI)
		if e.Resource != nil {
			parts = append(parts, e.Resource.Namespace, e.Resource.Name)
		}
	}
	for _, e := range report.Timeline {
		parts = append(parts, e.Message, e.Reason)
		if e.Resource != nil {
			parts = append(parts, e.Resource.Namespace)
		}
	}
	blob := strings.Join(parts, " ")

	if suspect = extractSuspectNS(fromNS, blob); suspect != "" {
		return suspect, fmt.Sprintf("dependency may involve namespace %q — need Coordinator verification", suspect), true
	}
	for _, u := range report.Unknowns {
		lu := strings.ToLower(u)
		if strings.Contains(lu, "outside") || strings.Contains(lu, "other namespace") || strings.Contains(lu, "cross-namespace") {
			return "", "dependency may be outside my namespace — need Coordinator verification", true
		}
	}
	lower := strings.ToLower(blob)
	if strings.Contains(lower, "other namespace") || strings.Contains(lower, "outside namespace") || strings.Contains(lower, "cross-namespace") {
		return "", "suspect dependency outside namespace", true
	}
	return "", "", false
}

func extractSuspectNS(fromNS, blob string) string {
	fromNS = strings.ToLower(strings.TrimSpace(fromNS))
	candidates := make([]string, 0, 4)
	for _, m := range reSvcDNS.FindAllStringSubmatch(blob, -1) {
		if len(m) > 1 {
			candidates = append(candidates, m[1])
		}
	}
	for _, m := range reNamespacePhrase.FindAllStringSubmatch(blob, -1) {
		if len(m) > 1 {
			candidates = append(candidates, m[1])
		}
	}
	for _, c := range candidates {
		ns := strings.ToLower(strings.TrimSpace(c))
		if ns == "" || ns == fromNS {
			continue
		}
		if ns == "svc" || ns == "cluster" || ns == "local" {
			continue
		}
		return ns
	}
	return ""
}

// FormatReply renders a compact human-readable CoordinatorReply for Slack / stdout (AG-053).
func FormatReply(r *Reply) string {
	if r == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "🔀 *Coordinator reply* · mutate=%v\n", r.MutateAttempted)
	if r.SuspectNamespace != "" {
		fmt.Fprintf(&b, "*Suspect namespace:* `%s`\n", r.SuspectNamespace)
	}
	if r.Reason != "" {
		fmt.Fprintf(&b, "*Handoff reason:* %s\n", r.Reason)
	}
	if r.Merged.Summary != "" {
		fmt.Fprintf(&b, "*Merged summary:* %s\n", r.Merged.Summary)
	}
	if r.Merged.Confidence > 0 {
		fmt.Fprintf(&b, "*Merged confidence:* %.0f%%\n", r.Merged.Confidence*100)
	}
	if n := len(r.Merged.Evidence); n > 0 {
		fmt.Fprintf(&b, "*Merged evidence:* %d refs\n", n)
	}
	if len(r.Routing) > 0 {
		fmt.Fprintf(&b, "*Routing:* %s\n", strings.Join(r.Routing, " · "))
	}
	for _, u := range r.Merged.Unknowns {
		u = strings.TrimSpace(u)
		if u == "" {
			continue
		}
		if strings.Contains(strings.ToLower(u), "coordinator") {
			fmt.Fprintf(&b, "• %s\n", u)
		}
	}
	fmt.Fprintf(&b, "_kprompt Coordinator · ADR-0017_")
	return b.String()
}
