package ui

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/safety"
)

func TestThemeDisabledIsPlain(t *testing.T) {
	th := palettes["dracula"]
	th.enabled = false
	if got := th.Heading("Intent:"); got != "Intent:" {
		t.Fatalf("disabled theme should not color: %q", got)
	}
	if got := th.Risk(safety.RiskHigh); got != "high" {
		t.Fatalf("disabled risk should be plain: %q", got)
	}
}

func TestThemeEnabledWrapsAnsi(t *testing.T) {
	th := palettes["auto"]
	th.enabled = true
	got := th.Success("ok")
	if !strings.HasPrefix(got, "\x1b[") || !strings.HasSuffix(got, ansiReset) {
		t.Fatalf("enabled theme should wrap in ANSI: %q", got)
	}
	if !strings.Contains(got, "ok") {
		t.Fatalf("styled text should contain original: %q", got)
	}
}

func TestRiskColorsBySeverity(t *testing.T) {
	th := palettes["auto"]
	th.enabled = true
	if !strings.Contains(th.Risk(safety.RiskLow), th.success) {
		t.Errorf("low risk should use success color")
	}
	if !strings.Contains(th.Risk(safety.RiskMedium), th.warn) {
		t.Errorf("medium risk should use warn color")
	}
	if !strings.Contains(th.Risk(safety.RiskDenied), th.danger) {
		t.Errorf("denied risk should use danger color")
	}
}

func TestColorEnabledRespectsNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("KPROMPT_FORCE_COLOR", "")
	if colorEnabled("dracula", &bytes.Buffer{}) {
		t.Fatal("NO_COLOR must disable color")
	}
}

func TestColorEnabledForceColor(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("KPROMPT_FORCE_COLOR", "1")
	if !colorEnabled("dracula", &bytes.Buffer{}) {
		t.Fatal("KPROMPT_FORCE_COLOR must enable color even for non-TTY writers")
	}
}

func TestColorEnabledDisableKeyword(t *testing.T) {
	t.Setenv("KPROMPT_FORCE_COLOR", "1")
	if colorEnabled("none", &bytes.Buffer{}) {
		t.Fatal("theme 'none' must force plain output")
	}
}

func TestNonTerminalBufferIsPlain(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	t.Setenv("KPROMPT_FORCE_COLOR", "")
	if colorEnabled("auto", &bytes.Buffer{}) {
		t.Fatal("a non-terminal writer should not be colored by default")
	}
}

func TestThemeForFallsBackToAuto(t *testing.T) {
	configuredTheme = "does-not-exist"
	defer func() { configuredTheme = "" }()
	th := themeFor(&bytes.Buffer{})
	if th.heading != palettes["auto"].heading {
		t.Fatal("unknown theme name should fall back to auto palette")
	}
}

func TestThemeNamesIncludeKnownPalettes(t *testing.T) {
	names := strings.Join(ThemeNames(), ",")
	for _, want := range []string{"auto", "dracula", "nord", "gruvbox", "mono", "none"} {
		if !strings.Contains(names, want) {
			t.Errorf("ThemeNames() missing %q: %s", want, names)
		}
	}
}
