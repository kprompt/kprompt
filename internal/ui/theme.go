package ui

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"text/tabwriter"

	"golang.org/x/term"

	"github.com/kprompt/kprompt/internal/safety"
)

const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiDim   = "\x1b[2m"
)

// Theme holds the resolved ANSI sequences for each semantic output role.
// A zero-value Theme (enabled=false) renders everything as plain text, so it is
// always safe to use even when colors are turned off.
type Theme struct {
	enabled bool
	heading string // section labels ("Intent:", "Actions:", table headers)
	success string // applied / low-risk / healthy
	warn    string // medium risk / warnings
	danger  string // denied / high risk / errors
	info    string // neutral informational values
	accent  string // commands, resource names, "Try:" hints
	muted   string // secondary text, diffs, events
	bold    string // emphasis without color
}

// configuredTheme is set from config/flags; empty means "resolve from env".
var configuredTheme string

// SetTheme selects the active palette by name (e.g. "auto", "dracula", "none").
// An empty name falls back to the KPROMPT_THEME env var, then "auto".
func SetTheme(name string) {
	configuredTheme = strings.ToLower(strings.TrimSpace(name))
}

func (t Theme) paint(code, text string) string {
	if !t.enabled || code == "" || text == "" {
		return text
	}
	return code + text + ansiReset
}

// Heading styles a section label.
func (t Theme) Heading(s string) string { return t.paint(t.heading, s) }

// Success styles positive/applied output.
func (t Theme) Success(s string) string { return t.paint(t.success, s) }

// Warn styles cautionary output.
func (t Theme) Warn(s string) string { return t.paint(t.warn, s) }

// Danger styles denied/high-risk/error output.
func (t Theme) Danger(s string) string { return t.paint(t.danger, s) }

// Info styles neutral values.
func (t Theme) Info(s string) string { return t.paint(t.info, s) }

// Accent styles commands and resource identifiers.
func (t Theme) Accent(s string) string { return t.paint(t.accent, s) }

// Muted styles secondary/low-emphasis text.
func (t Theme) Muted(s string) string { return t.paint(t.muted, s) }

// Bold emphasizes text without changing color.
func (t Theme) Bold(s string) string { return t.paint(t.bold, s) }

// tabHeading styles a tabwriter header row. The ANSI sequences are wrapped in
// tabwriter.Escape bytes so their width is ignored during column alignment,
// while the embedded tab separators remain outside the guards.
func (t Theme) tabHeading(text string) string {
	if !t.enabled || t.heading == "" {
		return text
	}
	esc := string([]byte{tabwriter.Escape})
	return esc + t.heading + esc + text + esc + ansiReset + esc
}

// Risk colors a safety risk level by severity.
func (t Theme) Risk(r safety.Risk) string {
	label := string(r)
	switch r {
	case safety.RiskLow:
		return t.paint(t.success, label)
	case safety.RiskMedium:
		return t.paint(t.warn, label)
	case safety.RiskHigh, safety.RiskDenied:
		return t.paint(t.danger, label)
	default:
		return t.paint(t.info, label)
	}
}

// Severity colors an arbitrary severity string from findings/suggestions.
func (t Theme) Severity(sev string) string {
	code := t.info
	switch strings.ToLower(strings.TrimSpace(sev)) {
	case "high", "critical", "error", "fatal", "danger":
		code = t.danger
	case "medium", "warn", "warning":
		code = t.warn
	case "low", "info", "notice":
		code = t.info
	}
	return t.paint(code, sev)
}

func rgb(r, g, b int) string { return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b) }

func rgbBold(r, g, b int) string { return ansiBold + rgb(r, g, b) }

// palettes maps theme names to their color sequences. The enabled flag is set
// later by themeFor based on TTY/NO_COLOR detection.
var palettes = map[string]Theme{
	// auto: 16-color palette for broad terminal compatibility.
	"auto": {
		heading: "\x1b[1;36m",
		success: "\x1b[32m",
		warn:    "\x1b[33m",
		danger:  "\x1b[31m",
		info:    "\x1b[36m",
		accent:  "\x1b[35m",
		muted:   "\x1b[90m",
		bold:    ansiBold,
	},
	// mono: structure via bold/dim only, no color.
	"mono": {
		heading: ansiBold,
		success: ansiBold,
		warn:    ansiBold,
		danger:  ansiBold,
		info:    "",
		accent:  ansiBold,
		muted:   ansiDim,
		bold:    ansiBold,
	},
	"dracula": {
		heading: rgbBold(189, 147, 249),
		success: rgb(80, 250, 123),
		warn:    rgb(241, 250, 140),
		danger:  rgb(255, 85, 85),
		info:    rgb(139, 233, 253),
		accent:  rgb(255, 121, 198),
		muted:   rgb(98, 114, 164),
		bold:    ansiBold,
	},
	"nord": {
		heading: rgbBold(129, 161, 193),
		success: rgb(163, 190, 140),
		warn:    rgb(235, 203, 139),
		danger:  rgb(191, 97, 106),
		info:    rgb(136, 192, 208),
		accent:  rgb(180, 142, 173),
		muted:   rgb(76, 86, 106),
		bold:    ansiBold,
	},
	"gruvbox": {
		heading: rgbBold(254, 128, 25),
		success: rgb(184, 187, 38),
		warn:    rgb(250, 189, 47),
		danger:  rgb(251, 73, 52),
		info:    rgb(131, 165, 152),
		accent:  rgb(211, 134, 155),
		muted:   rgb(146, 131, 116),
		bold:    ansiBold,
	},
}

// disableKeywords are theme names that force plain output.
var disableKeywords = map[string]bool{
	"none": true, "off": true, "no": true, "plain": true, "disable": true, "disabled": true,
}

// ThemeNames returns the selectable palette names (sorted) plus the disable alias.
func ThemeNames() []string {
	names := make([]string, 0, len(palettes)+1)
	for name := range palettes {
		names = append(names, name)
	}
	names = append(names, "none")
	sort.Strings(names)
	return names
}

// themeFor resolves the active theme for a writer, honoring configuration,
// KPROMPT_THEME, NO_COLOR, KPROMPT_FORCE_COLOR, and TTY detection.
func themeFor(w io.Writer) Theme {
	name := configuredTheme
	if name == "" {
		name = strings.ToLower(strings.TrimSpace(os.Getenv("KPROMPT_THEME")))
	}
	if name == "" {
		name = "auto"
	}
	p, ok := palettes[name]
	if !ok {
		p = palettes["auto"]
	}
	p.enabled = colorEnabled(name, w)
	return p
}

func colorEnabled(name string, w io.Writer) bool {
	if disableKeywords[name] {
		return false
	}
	// Per the NO_COLOR convention, any non-empty value disables color.
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	if os.Getenv("KPROMPT_FORCE_COLOR") != "" {
		return true
	}
	return isTerminalWriter(w)
}

func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}
