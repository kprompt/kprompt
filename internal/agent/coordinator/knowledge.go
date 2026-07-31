package coordinator

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

// KnowledgeEdge is one observed from→suspect handoff relationship (AG-059).
type KnowledgeEdge struct {
	From    string `json:"from"`
	Suspect string `json:"suspect,omitempty"`
	Count   int    `json:"count"`
	LastAt  string `json:"lastAt,omitempty"`
}

// KnowledgeSummary is the Shared Knowledge view over recent handoffs.
// With a Store (AG-060) Durable is true; still not a full blast-radius product graph.
type KnowledgeSummary struct {
	APIVersion      string          `json:"apiVersion"`
	Kind            string          `json:"kind"`
	SchemaVersion   string          `json:"schemaVersion"`
	GeneratedAt     time.Time       `json:"generatedAt"`
	HandoffCount    int             `json:"handoffCount"`
	Namespaces      []string        `json:"namespaces,omitempty"`
	Edges           []KnowledgeEdge `json:"edges,omitempty"`
	LatestSummaries []string        `json:"latestSummaries,omitempty"`
	Durable         bool            `json:"durable"`
	Note            string          `json:"note,omitempty"`
}

const kindKnowledge = "CoordinatorKnowledge"

// Summarize builds Shared Knowledge from the recent ring.
func Summarize(records []Record, durable bool) KnowledgeSummary {
	nsSet := map[string]struct{}{}
	type edgeKey struct{ from, suspect string }
	edges := map[edgeKey]*KnowledgeEdge{}
	var latest []string

	for _, rec := range records {
		from := strings.TrimSpace(rec.Envelope.FromNamespace)
		suspect := strings.TrimSpace(rec.Envelope.SuspectNamespace)
		if from != "" {
			nsSet[from] = struct{}{}
		}
		if suspect != "" {
			nsSet[suspect] = struct{}{}
		}
		key := edgeKey{from: from, suspect: suspect}
		e, ok := edges[key]
		if !ok {
			e = &KnowledgeEdge{From: from, Suspect: suspect}
			edges[key] = e
		}
		e.Count++
		at := rec.At
		if at.IsZero() {
			at = rec.Reply.CreatedAt
		}
		if !at.IsZero() {
			e.LastAt = at.UTC().Format(time.RFC3339)
		}
		sum := strings.TrimSpace(rec.Reply.Merged.Summary)
		if sum == "" {
			sum = strings.TrimSpace(rec.Envelope.Report.Summary)
		}
		if sum != "" {
			label := from
			if suspect != "" {
				label = from + "→" + suspect
			}
			latest = append(latest, fmt.Sprintf("%s: %s", label, sum))
		}
	}

	ns := make([]string, 0, len(nsSet))
	for n := range nsSet {
		ns = append(ns, n)
	}
	sort.Strings(ns)

	edgeList := make([]KnowledgeEdge, 0, len(edges))
	for _, e := range edges {
		edgeList = append(edgeList, *e)
	}
	sort.Slice(edgeList, func(i, j int) bool {
		if edgeList[i].Count != edgeList[j].Count {
			return edgeList[i].Count > edgeList[j].Count
		}
		if edgeList[i].From != edgeList[j].From {
			return edgeList[i].From < edgeList[j].From
		}
		return edgeList[i].Suspect < edgeList[j].Suspect
	})

	const maxLatest = 10
	if len(latest) > maxLatest {
		latest = latest[len(latest)-maxLatest:]
	}

	note := "Shared Knowledge MVP: in-memory recent handoffs only (restart-lossy); not a full blast-radius graph"
	if durable {
		note = "Shared Knowledge: durable handoff ring (file/ConfigMap); not a full continuous blast-radius product graph"
	}

	return KnowledgeSummary{
		APIVersion:      APIVersion,
		Kind:            kindKnowledge,
		SchemaVersion:   SchemaVersion,
		GeneratedAt:     time.Now().UTC(),
		HandoffCount:    len(records),
		Namespaces:      ns,
		Edges:           edgeList,
		LatestSummaries: latest,
		Durable:         durable,
		Note:            note,
	}
}

// FormatKnowledge renders a compact human-readable Shared Knowledge summary.
func FormatKnowledge(k KnowledgeSummary) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Coordinator Shared Knowledge (MVP) · durable=%v\n", k.Durable)
	fmt.Fprintf(&b, "Handoffs remembered: %d\n", k.HandoffCount)
	if len(k.Namespaces) > 0 {
		fmt.Fprintf(&b, "Namespaces seen: %s\n", strings.Join(k.Namespaces, ", "))
	}
	for _, e := range k.Edges {
		if e.Suspect == "" {
			fmt.Fprintf(&b, "- %s (no suspect) x%d\n", e.From, e.Count)
			continue
		}
		fmt.Fprintf(&b, "- %s -> %s x%d\n", e.From, e.Suspect, e.Count)
	}
	for _, s := range k.LatestSummaries {
		fmt.Fprintf(&b, "  - %s\n", s)
	}
	if k.Note != "" {
		fmt.Fprintf(&b, "%s\n", k.Note)
	}
	return strings.TrimSpace(b.String())
}

func (h *Handler) knowledge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "GET only", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.Service.Knowledge())
}
