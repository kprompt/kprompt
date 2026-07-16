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
	t := themeFor(out)
	fmt.Fprint(out, t.Bold("Apply this plan?")+" [y/N]: ")
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

// PrintAborted notes the user declined to apply.
func PrintAborted(w io.Writer) {
	t := themeFor(w)
	fmt.Fprintln(w, t.Muted("Aborted."))
}
