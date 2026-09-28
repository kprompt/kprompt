// Package xdg resolves XDG Base Directory paths without hardcoding ~/.config.
package xdg

import (
	"os"
	"path/filepath"
	"strings"
)

// ConfigHome returns $XDG_CONFIG_HOME, or ~/.config when unset (XDG default).
// Does not use os.UserConfigDir so macOS stays on ~/.config rather than
// ~/Library/Application Support (matches Linux tooling expectations).
func ConfigHome() string {
	if v := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); v != "" {
		return filepath.Clean(v)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(".", ".config")
	}
	return filepath.Join(home, ".config")
}

// KpromptDir returns $XDG_CONFIG_HOME/kprompt[/parts...] (or ~/.config/kprompt/…).
func KpromptDir(parts ...string) string {
	base := filepath.Join(ConfigHome(), "kprompt")
	if len(parts) == 0 {
		return base
	}
	return filepath.Join(append([]string{base}, parts...)...)
}
