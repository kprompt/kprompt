// Package demo implements `kprompt demo` — Observe walkthrough entry (OB-004).
package demo

import (
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/kprompt/kprompt/internal/ui"
)

// ExamplesRepo is the documented walkthrough repository.
const ExamplesRepo = "https://github.com/kprompt/kprompt-examples.git"

// Check is one prerequisite row.
type Check struct {
	Name   string
	OK     bool
	Detail string
	Hint   string
}

// Options configures Run.
type Options struct {
	CheckOnly bool
	Out       io.Writer
	// LookPath overrides exec.LookPath (tests).
	LookPath func(file string) (string, error)
}

// Run prints the Observe demo guide and prerequisite status.
// MVP: guided checklist (does not clone/run make). Use printed commands to execute.
func Run(opts Options) error {
	if opts.Out == nil {
		opts.Out = io.Discard
	}
	look := opts.LookPath
	if look == nil {
		look = exec.LookPath
	}

	checks := []Check{
		checkTool(look, "docker", "Docker (or colima/Podman providing docker CLI)", "https://docs.docker.com/get-docker/"),
		checkTool(look, "kind", "kind", "brew install kind"),
		checkTool(look, "kubectl", "kubectl", "brew install kubectl"),
		checkTool(look, "make", "make", "install make (Xcode CLT / build-essential)"),
		checkTool(look, "git", "git", "install git"),
		checkTool(look, "kprompt", "kprompt on PATH", "curl -fsSL https://kprompt.ai/install | bash"),
	}

	t := ui.ThemeForWriter(opts.Out)
	fmt.Fprintln(opts.Out, t.Bold("kprompt demo")+" — Observe walkthrough ($0, no LLM key)")
	fmt.Fprintln(opts.Out, "")
	fmt.Fprintln(opts.Out, "This is the Observe agent demo (heuristic): kind cluster, broken workloads, propose-only.")
	fmt.Fprintln(opts.Out, "It is not the NL plan→approve loop. For that: kprompt init --ollama then kprompt \"list pods\".")
	fmt.Fprintln(opts.Out, "")

	fmt.Fprintln(opts.Out, "Prerequisites:")
	allOK := true
	for _, c := range checks {
		mark := t.Danger("✗")
		if c.OK {
			mark = t.Success("✓")
		} else {
			allOK = false
		}
		fmt.Fprintf(opts.Out, "  %s  %-8s  %s\n", mark, c.Name, c.Detail)
		if !c.OK && c.Hint != "" {
			fmt.Fprintf(opts.Out, "         → %s\n", c.Hint)
		}
	}
	fmt.Fprintln(opts.Out, "")

	if opts.CheckOnly {
		if !allOK {
			return fmt.Errorf("demo: missing prerequisites")
		}
		fmt.Fprintln(opts.Out, "All prerequisites found.")
		return nil
	}

	fmt.Fprintln(opts.Out, "Run the walkthrough:")
	fmt.Fprintln(opts.Out, "")
	fmt.Fprintf(opts.Out, "  git clone %s\n", ExamplesRepo)
	fmt.Fprintln(opts.Out, "  cd kprompt-examples && make walkthrough")
	fmt.Fprintln(opts.Out, "")
	fmt.Fprintln(opts.Out, "One failure at a time:")
	fmt.Fprintln(opts.Out, "  make up && make break SCENARIO=01-crashloop && make agent")
	fmt.Fprintln(opts.Out, "")
	if !allOK {
		fmt.Fprintln(opts.Out, t.Warn("Install missing tools above, then re-run: kprompt demo --check"))
		fmt.Fprintln(opts.Out, "")
	}
	fmt.Fprintln(opts.Out, "When you are done exploring Observe:")
	fmt.Fprintln(opts.Out, `  kprompt init --ollama`)
	fmt.Fprintln(opts.Out, `  kprompt "how's my cluster"`)
	fmt.Fprintln(opts.Out, `  kprompt "scale api to 3"   # plan first, then y/N`)
	return nil
}

func checkTool(look func(string) (string, error), name, label, hint string) Check {
	path, err := look(name)
	if err != nil || strings.TrimSpace(path) == "" {
		return Check{Name: name, OK: false, Detail: "not found", Hint: hint}
	}
	return Check{Name: name, OK: true, Detail: path}
}
