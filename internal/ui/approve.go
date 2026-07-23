package ui

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// StdinIsTerminal reports whether stdin is an interactive TTY.
func StdinIsTerminal() bool {
	return term.IsTerminal(int(os.Stdin.Fd()))
}

// ConfirmApply prompts `Apply this plan? [y/N]` and returns true for y/yes.
// Empty input, n/no, or EOF aborts (false, nil).
func ConfirmApply(in io.Reader, out io.Writer) (bool, error) {
	return ConfirmApplyContext(in, out, "")
}

// ConfirmApplyContext prompts apply for a specific kube context (multi-mutate fan-out).
func ConfirmApplyContext(in io.Reader, out io.Writer, contextName string) (bool, error) {
	t := themeFor(out)
	prompt := t.Bold("Apply this plan?") + " [y/N]: "
	if strings.TrimSpace(contextName) != "" {
		prompt = t.Bold(fmt.Sprintf("Apply this plan to context %q?", contextName)) + " [y/N]: "
	}
	fmt.Fprint(out, prompt)
	if f, ok := out.(*os.File); ok {
		_ = f.Sync()
	}
	reader := bufio.NewReader(in)
	line, err := reader.ReadString('\n')
	if err != nil && err != io.EOF {
		return false, err
	}
	answer := strings.TrimSpace(strings.ToLower(line))
	switch answer {
	case "y", "yes":
		return true, nil
	default:
		return false, nil
	}
}

// PrintNeedsApprove reminds non-interactive users to pass --approve.
func PrintNeedsApprove(w io.Writer) {
	t := themeFor(w)
	fmt.Fprintln(w, t.Warn("Mutation requires approval. Re-run with --approve, or run in a TTY to confirm interactively."))
}

// PrintNeedsApproveEachContext explains why plain --approve is refused for multi-context mutate.
func PrintNeedsApproveEachContext(w io.Writer) {
	t := themeFor(w)
	fmt.Fprintln(w, t.Warn("Multi-context mutate refuses a single --approve. Confirm each context in a TTY, or pass --approve-each-context (applies the same plan to every listed context)."))
}

// PrintAborted notes the user declined to apply.
func PrintAborted(w io.Writer) {
	t := themeFor(w)
	fmt.Fprintln(w, t.Muted("Aborted."))
}
