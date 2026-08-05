package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestCommandGroups(t *testing.T) {
	root := &cobra.Command{Use: "kprompt"}
	registerCommandGroups(root)
	day0 := withGroup(&cobra.Command{Use: "init", Short: "init"}, groupDay0)
	adv := withGroup(&cobra.Command{Use: "agent", Short: "agent"}, groupAdvanced)
	root.AddCommand(day0, adv, newAdvancedHelpCmd(root))

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Day-0 Commands:") {
		t.Fatalf("missing Day-0 group:\n%s", out)
	}
	if !strings.Contains(out, "Advanced Commands:") {
		t.Fatalf("missing Advanced group:\n%s", out)
	}
}

func TestAdvancedHelpLists(t *testing.T) {
	root := &cobra.Command{Use: "kprompt"}
	registerCommandGroups(root)
	root.AddCommand(withGroup(&cobra.Command{Use: "setup", Short: "bootstrap"}, groupAdvanced))
	root.AddCommand(newAdvancedHelpCmd(root))

	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"advanced"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "setup") {
		t.Fatalf("out=%s", buf.String())
	}
}
