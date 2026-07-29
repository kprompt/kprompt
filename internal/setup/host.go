package setup

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// HostResult is the outcome of applying one host-lane step (T-063).
type HostResult struct {
	Component string `json:"component"`
	Status    string `json:"status"` // installed | skipped | failed | unsupported
	Detail    string `json:"detail,omitempty"`
	Method    string `json:"method,omitempty"` // brew | get-helm-3 | …
}

// ApplyReport summarizes host install attempts.
type ApplyReport struct {
	Applied []HostResult `json:"applied"`
	Notes   []string     `json:"notes,omitempty"`
}

// CommandRunner executes host install commands (injectable for tests).
type CommandRunner interface {
	LookPath(file string) (string, error)
	Run(ctx context.Context, name string, args []string, env []string) error
	TempDir() string
	GOOS() string
}

// DefaultRunner shells out via os/exec.
type DefaultRunner struct{}

func (DefaultRunner) LookPath(file string) (string, error) { return exec.LookPath(file) }

func (DefaultRunner) Run(ctx context.Context, name string, args []string, env []string) error {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), env...)
	}
	return cmd.Run()
}

func (DefaultRunner) TempDir() string { return os.TempDir() }
func (DefaultRunner) GOOS() string    { return runtime.GOOS }

// InstallMethod is how we would install a host binary on this OS.
type InstallMethod struct {
	ID          string // brew | get-helm-3 | unsupported
	Description string
	Binary      string // expected PATH name after install
	Prepare     func(ctx context.Context, r CommandRunner) (name string, args []string, cleanup func(), err error)
}

// HostNeeded returns host-lane steps that still need install.
func HostNeeded(plan Plan) []Step {
	out := make([]Step, 0)
	for _, s := range plan.Steps {
		if s.Lane == LaneHost && s.Status == StatusNeeded {
			out = append(out, s)
		}
	}
	return out
}

// ResolveHelmMethod picks a safe install path for the current OS.
func ResolveHelmMethod(r CommandRunner) (InstallMethod, error) {
	goos := r.GOOS()
	switch goos {
	case "darwin":
		if _, err := r.LookPath("brew"); err == nil {
			return brewHelmMethod(), nil
		}
		return InstallMethod{
			ID:          "unsupported",
			Description: "macOS: install Homebrew, then: brew install helm — or see https://helm.sh/docs/intro/install/",
			Binary:      "helm",
		}, nil
	case "linux":
		if _, err := r.LookPath("brew"); err == nil {
			return brewHelmMethod(), nil
		}
		if _, err := r.LookPath("curl"); err != nil {
			return InstallMethod{
				ID:          "unsupported",
				Description: "Linux: install curl, then re-run, or follow https://helm.sh/docs/intro/install/",
				Binary:      "helm",
			}, nil
		}
		return getHelm3Method(), nil
	default:
		return InstallMethod{
			ID:          "unsupported",
			Description: fmt.Sprintf("%s is not in the supported OS matrix — install Helm manually: https://helm.sh/docs/intro/install/", goos),
			Binary:      "helm",
		}, nil
	}
}

func brewHelmMethod() InstallMethod {
	return InstallMethod{
		ID:          "brew",
		Description: "brew install helm",
		Binary:      "helm",
		Prepare: func(context.Context, CommandRunner) (string, []string, func(), error) {
			return "brew", []string{"install", "helm"}, func() {}, nil
		},
	}
}

func getHelm3Method() InstallMethod {
	return InstallMethod{
		ID:          "get-helm-3",
		Description: "official get-helm-3 script (https://helm.sh/docs/intro/install/)",
		Binary:      "helm",
		Prepare: func(ctx context.Context, r CommandRunner) (string, []string, func(), error) {
			dir, err := os.MkdirTemp(r.TempDir(), "kprompt-helm-*")
			if err != nil {
				return "", nil, nil, err
			}
			script := filepath.Join(dir, "get-helm-3")
			cleanup := func() { _ = os.RemoveAll(dir) }
			// Download script with curl (already required).
			if err := r.Run(ctx, "curl", []string{
				"-fsSL",
				"-o", script,
				"https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3",
			}, nil); err != nil {
				cleanup()
				return "", nil, nil, fmt.Errorf("download get-helm-3: %w", err)
			}
			if err := os.Chmod(script, 0o700); err != nil {
				cleanup()
				return "", nil, nil, err
			}
			return script, nil, cleanup, nil
		},
	}
}

// ApplyHost installs missing host CLIs from the plan (T-063).
// Skips components already on PATH. Does not touch cluster/config lanes.
func ApplyHost(ctx context.Context, plan Plan, r CommandRunner, out io.Writer) (ApplyReport, error) {
	if r == nil {
		r = DefaultRunner{}
	}
	rep := ApplyReport{Notes: []string{
		"Host apply only. Cluster operators remain a separate step; URL config stays manual / config set.",
	}}
	needed := HostNeeded(plan)
	if len(needed) == 0 {
		rep.Notes = append(rep.Notes, "No host-lane steps needed.")
		return rep, nil
	}

	for _, step := range needed {
		if step.Component != "helm" && step.ID != "helm" {
			rep.Applied = append(rep.Applied, HostResult{
				Component: step.Component,
				Status:    "unsupported",
				Detail:    "Setup currently installs Helm only; other host CLIs are not wired yet",
			})
			continue
		}
		res := installHelm(ctx, r, out)
		rep.Applied = append(rep.Applied, res)
		if res.Status == "failed" {
			return rep, fmt.Errorf("helm install failed: %s", res.Detail)
		}
	}
	return rep, nil
}

func installHelm(ctx context.Context, r CommandRunner, out io.Writer) HostResult {
	if path, err := r.LookPath("helm"); err == nil {
		return HostResult{
			Component: "helm",
			Status:    "skipped",
			Detail:    "already on PATH: " + path,
			Method:    "none",
		}
	}
	method, err := ResolveHelmMethod(r)
	if err != nil {
		return HostResult{Component: "helm", Status: "failed", Detail: err.Error()}
	}
	if method.ID == "unsupported" || method.Prepare == nil {
		return HostResult{
			Component: "helm",
			Status:    "unsupported",
			Detail:    method.Description,
			Method:    method.ID,
		}
	}
	fmt.Fprintf(out, "Installing helm via %s…\n", method.ID)
	name, args, cleanup, err := method.Prepare(ctx, r)
	if cleanup != nil {
		defer cleanup()
	}
	if err != nil {
		return HostResult{Component: "helm", Status: "failed", Detail: err.Error(), Method: method.ID}
	}
	runCtx := ctx
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
	}
	if err := r.Run(runCtx, name, args, nil); err != nil {
		return HostResult{
			Component: "helm",
			Status:    "failed",
			Detail:    err.Error(),
			Method:    method.ID,
		}
	}
	path, err := r.LookPath("helm")
	detail := method.Description
	if err == nil {
		detail = "installed: " + path
	} else {
		detail = "install finished but helm not yet on PATH — open a new shell or check installer output"
	}
	return HostResult{
		Component: "helm",
		Status:    "installed",
		Detail:    detail,
		Method:    method.ID,
	}
}

// FormatApply writes host apply results for humans.
func FormatApply(w io.Writer, rep ApplyReport) {
	fmt.Fprintln(w, "\nHost apply:")
	if len(rep.Applied) == 0 {
		fmt.Fprintln(w, "  (nothing)")
	}
	for _, a := range rep.Applied {
		line := fmt.Sprintf("  - [%s] %s", a.Status, a.Component)
		if a.Method != "" && a.Method != "none" {
			line += " via " + a.Method
		}
		if a.Detail != "" {
			line += ": " + a.Detail
		}
		fmt.Fprintln(w, line)
	}
	for _, n := range rep.Notes {
		fmt.Fprintf(w, "  note: %s\n", n)
	}
}

// OSMatrixDoc is the supported install matrix for docs / --help.
func OSMatrixDoc() string {
	var b strings.Builder
	b.WriteString("Host install OS matrix:\n")
	b.WriteString("  darwin  — Homebrew: brew install helm (brew required)\n")
	b.WriteString("  linux   — brew if present, else official get-helm-3 script (curl required)\n")
	b.WriteString("  other   — unsupported; install manually (https://helm.sh/docs/intro/install/)\n")
	b.WriteString("Skip if helm already on PATH. Cluster installs are a separate step.\n")
	return b.String()
}
