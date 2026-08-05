// Package coach prints Day-0 readiness guidance for bare `kprompt` (OB-001).
package coach

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/tools"
	"github.com/kprompt/kprompt/internal/ui"
)

// Status is a compact readiness snapshot for the empty-invoke coach.
type Status struct {
	Version string

	KubeOK     bool
	KubeDetail string

	LLMOK     bool
	LLMDetail string

	ClusterOK     bool
	ClusterDetail string
}

// Ready is true when LLM is configured (and keyed if needed) and the cluster is reachable.
func (s Status) Ready() bool {
	return s.LLMOK && s.ClusterOK
}

// Options configures Gather.
type Options struct {
	// Detect overrides tools.Detect (tests).
	Detect func(ctx context.Context, opts tools.DetectOptions) (*tools.Registry, error)
	// CurrentContext overrides cluster.CurrentContext (tests).
	CurrentContext func() (string, error)
}

// Gather builds Status from local config + kube detect.
func Gather(ctx context.Context, version string, opts Options) (Status, error) {
	file, err := config.LoadFile()
	if err != nil {
		return Status{}, err
	}
	s := Status{Version: version}

	curCtx := opts.CurrentContext
	if curCtx == nil {
		curCtx = cluster.CurrentContext
	}
	if name, err := curCtx(); err == nil && strings.TrimSpace(name) != "" {
		s.KubeOK = true
		s.KubeDetail = name
	} else {
		s.KubeDetail = "no kubeconfig"
		if err != nil {
			s.KubeDetail = shortErr(err)
		}
	}

	s.LLMOK, s.LLMDetail = llmStatus(file)

	detect := opts.Detect
	if detect == nil {
		detect = tools.Detect
	}
	reg, err := detect(ctx, tools.DetectOptions{
		Context: first(file.Context),
		File:    file,
	})
	if err != nil {
		s.ClusterOK = false
		s.ClusterDetail = shortErr(err)
		return s, nil
	}
	if r, ok := reg.Get(tools.IDKubernetes); ok && r.Available() {
		s.ClusterOK = true
		s.ClusterDetail = "reachable"
		if detail := strings.TrimSpace(r.Detail); detail != "" {
			s.ClusterDetail = detail
		}
	} else {
		s.ClusterOK = false
		s.ClusterDetail = "unreachable"
		if r, ok := reg.Get(tools.IDKubernetes); ok {
			if d := strings.TrimSpace(r.Detail); d != "" {
				s.ClusterDetail = d
			}
			if h := strings.TrimSpace(r.Hint); h != "" {
				s.ClusterDetail = h
			}
		}
	}
	return s, nil
}

func llmStatus(file config.File) (ok bool, detail string) {
	if strings.TrimSpace(file.Provider) == "" {
		return false, "no provider configured"
	}
	r := config.Merge(file, "", "", "", "", false, "")
	preset, found := llm.LookupPreset(r.Provider)
	if !found {
		return false, fmt.Sprintf("unknown provider %q", r.Provider)
	}
	if preset.AllowEmptyKey {
		return true, fmt.Sprintf("%s · %s", r.Provider, r.Model)
	}
	if config.APIKeyFor(r.Provider) == "" {
		return false, fmt.Sprintf("%s · API key unset", r.Provider)
	}
	return true, fmt.Sprintf("%s · %s", r.Provider, r.Model)
}

// Format writes the coach message to w.
func Format(w io.Writer, s Status) error {
	t := ui.ThemeForWriter(w)
	headline := fmt.Sprintf("kprompt %s", s.Version)
	if s.Version == "" {
		headline = "kprompt"
	}
	if s.Ready() {
		fmt.Fprintf(w, "%s · ready for natural language\n\n", t.Success(headline))
	} else {
		fmt.Fprintf(w, "%s · not ready for natural language yet\n\n", t.Warn(headline))
	}

	fmt.Fprintf(w, "  %-11s %s  %s\n", "kubeconfig", mark(t, s.KubeOK), s.KubeDetail)
	fmt.Fprintf(w, "  %-11s %s  %s\n", "llm", mark(t, s.LLMOK), s.LLMDetail)
	fmt.Fprintf(w, "  %-11s %s  %s\n\n", "cluster", mark(t, s.ClusterOK), s.ClusterDetail)

	if s.Ready() {
		fmt.Fprintln(w, "Try:")
		fmt.Fprintln(w, `  kprompt "list pods"`)
		fmt.Fprintln(w, `  kprompt "how's my cluster"`)
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "More: kprompt doctor · kprompt --help")
		return nil
	}

	fmt.Fprintln(w, "Next (pick one):")
	n := 1
	if !s.LLMOK {
		fmt.Fprintf(w, "  %d. $0 local LLM     kprompt init --ollama\n", n)
		n++
		fmt.Fprintf(w, "  %d. BYOK             kprompt init --provider openai\n", n)
		n++
	}
	if !s.ClusterOK || !s.KubeOK {
		fmt.Fprintf(w, "  %d. Fix kubeconfig   kubectl cluster-info  (or set KUBECONFIG)\n", n)
		n++
	}
	fmt.Fprintf(w, "  %d. Observe demo     kprompt demo\n", n)
	fmt.Fprintln(w, "")
	if s.ClusterOK {
		fmt.Fprintln(w, `Or try without LLM:  kprompt "how's my cluster"`)
	}
	fmt.Fprintln(w, "Help: kprompt --help · kprompt doctor · kprompt advanced")
	return nil
}

// FormatBrief is a one-screen non-TTY hint.
func FormatBrief(w io.Writer, s Status) error {
	if s.Ready() {
		fmt.Fprintln(w, `kprompt: ready — try: kprompt "list pods"`)
		return nil
	}
	fmt.Fprintln(w, "kprompt: not ready — run: kprompt init --ollama")
	fmt.Fprintln(w, "  or: kprompt doctor")
	return nil
}

func mark(t ui.Theme, ok bool) string {
	if ok {
		return t.Success("✓")
	}
	return t.Danger("✗")
}

func shortErr(err error) string {
	msg := err.Error()
	if i := strings.Index(msg, "\n"); i >= 0 {
		msg = msg[:i]
	}
	if len(msg) > 80 {
		return msg[:77] + "..."
	}
	return msg
}

func first(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
