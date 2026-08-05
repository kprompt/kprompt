// Package onboard implements `kprompt init` (OB-003).
package onboard

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/contexts"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/ui"
)

// Options configures Run.
type Options struct {
	Provider string // explicit provider id
	Ollama   bool   // shortcut for provider=ollama
	Model    string
	Context  string // kube context to persist (optional)
	DryRun   bool
	// Interactive forces/disables prompts. nil = use ui.StdinIsTerminal().
	Interactive *bool
	In          io.Reader
	Out         io.Writer
	// HTTPClient for optional Ollama reachability check (tests).
	HTTPClient *http.Client
}

// Result is what would be / was written.
type Result struct {
	Provider string
	Model    string
	Context  string
	Path     string
	Wrote    bool
}

// Run configures ~/.kprompt/config.yaml for Day-0 NL readiness.
func Run(ctx context.Context, opts Options) (Result, error) {
	if opts.In == nil {
		opts.In = strings.NewReader("")
	}
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	interactive := ui.StdinIsTerminal()
	if opts.Interactive != nil {
		interactive = *opts.Interactive
	}

	prov := strings.ToLower(strings.TrimSpace(opts.Provider))
	if opts.Ollama {
		prov = "ollama"
	}

	if prov == "" {
		if !interactive {
			return Result{}, fmt.Errorf("non-interactive init requires --ollama or --provider <name>\n  example: kprompt init --ollama")
		}
		fmt.Fprintln(opts.Out, "Configure kprompt for natural-language plans.")
		fmt.Fprintln(opts.Out, "Providers: ollama ($0 local) · openai · anthropic · gemini · …")
		choice, err := promptLine(opts.In, opts.Out, "Provider [ollama]: ")
		if err != nil {
			return Result{}, err
		}
		prov = strings.ToLower(strings.TrimSpace(choice))
		if prov == "" {
			prov = "ollama"
		}
	}

	preset, ok := llm.LookupPreset(prov)
	if !ok {
		return Result{}, fmt.Errorf("unknown provider %q (supported: %s)", prov, llm.SupportedNames())
	}

	model := strings.TrimSpace(opts.Model)
	if model == "" {
		model = preset.DefaultModel
		if interactive && opts.Provider == "" && !opts.Ollama {
			// Only prompt when fully interactive wizard (no flags).
			hint := model
			line, err := promptLine(opts.In, opts.Out, fmt.Sprintf("Model [%s]: ", hint))
			if err != nil {
				return Result{}, err
			}
			if strings.TrimSpace(line) != "" {
				model = strings.TrimSpace(line)
			}
		}
	}

	kubeCtx := strings.TrimSpace(opts.Context)
	if kubeCtx == "" && interactive {
		kubeCtx = pickContext(ctx, opts)
	}

	path, err := config.DefaultPath()
	if err != nil {
		return Result{}, err
	}

	res := Result{Provider: prov, Model: model, Context: kubeCtx, Path: path}
	fmt.Fprintf(opts.Out, "\nWill set:\n  provider: %s\n  model:    %s\n", prov, model)
	if kubeCtx != "" {
		fmt.Fprintf(opts.Out, "  context:  %s\n", kubeCtx)
	}
	fmt.Fprintf(opts.Out, "  file:     %s\n", path)

	if opts.DryRun {
		fmt.Fprintln(opts.Out, "\nDry-run — nothing written.")
		return res, nil
	}

	if _, err := config.SetField("provider", prov); err != nil {
		return res, err
	}
	if _, err := config.SetField("model", model); err != nil {
		return res, err
	}
	if kubeCtx != "" {
		if _, err := config.SetField("context", kubeCtx); err != nil {
			return res, err
		}
	}
	res.Wrote = true

	fmt.Fprintln(opts.Out, "\nSaved.")
	if !preset.AllowEmptyKey {
		primary := "API key"
		if len(preset.EnvKeys) > 0 {
			primary = preset.EnvKeys[0]
		}
		fmt.Fprintf(opts.Out, "Export your key before NL prompts:\n  export %s=...\n", primary)
		if preset.HelpURL != "" {
			fmt.Fprintf(opts.Out, "Keys: %s\n", preset.HelpURL)
		}
	} else {
		checkOllama(ctx, opts)
		fmt.Fprintln(opts.Out, "Ollama needs a local model, e.g.:")
		fmt.Fprintf(opts.Out, "  ollama serve && ollama pull %s\n", model)
	}

	fmt.Fprintln(opts.Out, "\nTry:")
	fmt.Fprintln(opts.Out, `  kprompt "how's my cluster"`)
	fmt.Fprintln(opts.Out, `  kprompt "list pods"`)
	fmt.Fprintln(opts.Out, "Recheck: kprompt doctor")
	return res, nil
}

func pickContext(ctx context.Context, opts Options) string {
	rep, err := contexts.List(ctx, contexts.Options{})
	if err != nil || len(rep.Items) == 0 {
		return ""
	}
	fmt.Fprintln(opts.Out, "\nKube contexts:")
	for i, e := range rep.Items {
		mark := " "
		if e.Current {
			mark = "*"
		}
		fmt.Fprintf(opts.Out, "  %d) %s %s\n", i+1, mark, e.Name)
	}
	line, err := promptLine(opts.In, opts.Out, "Context number or name [Enter=skip]: ")
	if err != nil || strings.TrimSpace(line) == "" {
		return ""
	}
	line = strings.TrimSpace(line)
	// numeric index
	var idx int
	if _, err := fmt.Sscanf(line, "%d", &idx); err == nil && idx >= 1 && idx <= len(rep.Items) {
		return rep.Items[idx-1].Name
	}
	return line
}

func checkOllama(ctx context.Context, opts Options) {
	client := opts.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 2 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:11434/api/tags", nil)
	if err != nil {
		return
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintln(opts.Out, "Note: Ollama does not look reachable at http://127.0.0.1:11434 — start it with: ollama serve")
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		fmt.Fprintln(opts.Out, "Ollama is reachable on localhost.")
	}
}

func promptLine(in io.Reader, out io.Writer, prompt string) (string, error) {
	fmt.Fprint(out, prompt)
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}
