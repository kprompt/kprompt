package history

import (
	"bufio"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/planner"
	"github.com/kprompt/kprompt/internal/safety"
)

const maxKeep = 200

// Disable skips Append/Truncate (used by unit tests).
var Disable bool

// Entry is one append-only history record (no secrets, no manifests).
type Entry struct {
	Time          time.Time `json:"time"`
	Prompt        string    `json:"prompt"`
	Kind          string    `json:"kind"`
	Summary       string    `json:"summary"`
	Namespace     string    `json:"namespace,omitempty"`
	Context       string    `json:"context,omitempty"`
	Risk          string    `json:"risk,omitempty"`
	Applied       bool      `json:"applied"`
	Actions       []string  `json:"actions,omitempty"`
	VerifyStatus  string    `json:"verify_status,omitempty"`
	VerifyMessage string    `json:"verify_message,omitempty"`
}

// DefaultPath returns ~/.kprompt/history.jsonl.
func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".kprompt", "history.jsonl"), nil
}

// FromPlan builds an Entry from a plan (never includes manifests or secrets).
func FromPlan(prompt, kubeContext string, plan planner.ExecutionPlan, risk safety.Result, applied bool) Entry {
	ns := ""
	actions := make([]string, 0, len(plan.Actions))
	for _, a := range plan.Actions {
		if ns == "" && a.Object.Namespace != "" {
			ns = a.Object.Namespace
		}
		line := fmt.Sprintf("%s %s/%s", a.Op, a.Object.Kind, a.Object.Name)
		if len(a.Command) > 0 {
			line = strings.Join(a.Command, " ")
		} else if a.Object.Namespace != "" {
			line += " -n " + a.Object.Namespace
		}
		actions = append(actions, line)
	}
	if ns == "" {
		ns = plan.Intent.Target.Namespace
	}
	return Entry{
		Time:      time.Now().UTC(),
		Prompt:    prompt,
		Kind:      string(plan.Intent.Kind),
		Summary:   plan.Summary,
		Namespace: ns,
		Context:   kubeContext,
		Risk:      string(risk.Risk),
		Applied:   applied,
		Actions:   actions,
	}
}

// Append writes one JSON line to the history file.
func Append(e Entry) error {
	if Disable || os.Getenv("KPROMPT_DISABLE_HISTORY") == "1" {
		return nil
	}
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	return enc.Encode(e)
}

// AppendPath is like Append but for a custom path (tests).
func AppendPath(path string, e Entry) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	return json.NewEncoder(f).Encode(e)
}

// List returns the newest entries first (up to limit).
func List(limit int) ([]Entry, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return ListPath(path, limit)
}

// ListPath lists history from a custom file (newest first).
func ListPath(path string, limit int) ([]Entry, error) {
	if limit <= 0 {
		limit = 20
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var all []Entry
	sc := bufio.NewScanner(f)
	// Support longer prompts.
	buf := make([]byte, 0, 64*1024)
	sc.Buffer(buf, 1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		all = append(all, e)
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	// Newest last on disk → reverse.
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	if len(all) > limit {
		all = all[:limit]
	}
	return all, nil
}

// Get returns the entry at 1-based index into List(limit) (1 = newest).
func Get(index, listLimit int) (Entry, error) {
	entries, err := List(listLimit)
	if err != nil {
		return Entry{}, err
	}
	if index < 1 || index > len(entries) {
		return Entry{}, fmt.Errorf("history index %d out of range (1-%d)", index, len(entries))
	}
	return entries[index-1], nil
}

// Truncate keeps only the newest maxKeep entries on disk (best-effort compaction).
func Truncate() error {
	if Disable || os.Getenv("KPROMPT_DISABLE_HISTORY") == "1" {
		return nil
	}
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	entries, err := ListPath(path, maxKeep*2)
	if err != nil || len(entries) <= maxKeep {
		return err
	}
	// ListPath already newest-first; keep maxKeep, rewrite oldest→newest.
	keep := entries[:maxKeep]
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for i := len(keep) - 1; i >= 0; i-- {
		if err := enc.Encode(keep[i]); err != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Clear removes all history entries by truncating/clearing the history file safely.
func Clear() error {
	if Disable || os.Getenv("KPROMPT_DISABLE_HISTORY") == "1" {
		return nil
	}
	path, err := DefaultPath()
	if err != nil {
		return err
	}
	return ClearPath(path)
}

// ClearPath clears a custom history file (used by tests).
func ClearPath(path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	return f.Close()
}

// FilterOptions contains criteria for filtering history entries.
type FilterOptions struct {
	Namespace string
	Kind      string
}

// ListFiltered returns newest entries filtered by the given criteria (up to limit).
func ListFiltered(limit int, opts FilterOptions) ([]Entry, error) {
	path, err := DefaultPath()
	if err != nil {
		return nil, err
	}
	return ListFilteredPath(path, limit, opts)
}

// ListFilteredPath lists filtered history from a custom file path.
func ListFilteredPath(path string, limit int, opts FilterOptions) ([]Entry, error) {
	entries, err := ListPath(path, math.MaxInt)
	if err != nil {
		return nil, err
	}

	maxResults := limit
	if limit <= 0 {
		maxResults = 20
	}

	var filtered []Entry
	targetNs := strings.ToLower(strings.TrimSpace(opts.Namespace))
	targetKind := strings.ToLower(strings.TrimSpace(opts.Kind))

	for _, e := range entries {
		if targetNs != "" && targetNs != strings.ToLower(e.Namespace) {
			continue
		}
		if targetKind != "" && targetKind != strings.ToLower(e.Kind) {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) == maxResults {
			break
		}
	}
	return filtered, nil
}

// FormatEntry renders a single detailed history entry for 'show'.
func FormatEntry(idx int, e Entry) string {
	var b strings.Builder
	applied := "plan"
	if e.Applied {
		applied = "applied"
	}
	if e.VerifyStatus != "" && e.VerifyStatus != "skipped" {
		applied += "/" + e.VerifyStatus
	}

	fmt.Fprintf(&b, "Entry #%d\n", idx)
	fmt.Fprintf(&b, "Time:      %s\n", e.Time.Local().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(&b, "Prompt:    %s\n", e.Prompt)
	if e.Summary != "" {
		fmt.Fprintf(&b, "Summary:   %s\n", e.Summary)
	}
	if e.Namespace != "" {
		fmt.Fprintf(&b, "Namespace: %s\n", e.Namespace)
	}
	if e.Context != "" {
		fmt.Fprintf(&b, "Context:   %s\n", e.Context)
	}
	if e.Kind != "" {
		fmt.Fprintf(&b, "Kind:      %s\n", e.Kind)
	}
	if e.Risk != "" {
		fmt.Fprintf(&b, "Risk:      %s\n", e.Risk)
	}
	fmt.Fprintf(&b, "Status:    %s\n", applied)

	if len(e.Actions) > 0 {
		b.WriteString("Actions:\n")
		for _, act := range e.Actions {
			fmt.Fprintf(&b, "  - %s\n", act)
		}
	}
	return b.String()
}

// FormatList renders a human-readable history list.
func FormatList(entries []Entry) string {
	if len(entries) == 0 {
		return "No history yet.\n"
	}
	var b strings.Builder
	for i, e := range entries {
		applied := "plan"
		if e.Applied {
			applied = "applied"
		}
		if e.VerifyStatus != "" && e.VerifyStatus != "skipped" {
			applied += "/" + e.VerifyStatus
		}
		ts := e.Time.Local().Format("2006-01-02 15:04")
		fmt.Fprintf(&b, "%2d. [%s] %s  %s\n", i+1, ts, e.Kind, applied)
		fmt.Fprintf(&b, "    %q\n", e.Prompt)
		if e.Summary != "" {
			fmt.Fprintf(&b, "    %s\n", e.Summary)
		}
	}
	return b.String()
}
