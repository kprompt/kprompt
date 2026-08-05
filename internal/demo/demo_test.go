package demo

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestRunCheckOnlyMissing(t *testing.T) {
	var out bytes.Buffer
	err := Run(Options{
		CheckOnly: true,
		Out:       &out,
		LookPath: func(file string) (string, error) {
			if file == "kubectl" {
				return "/usr/bin/kubectl", nil
			}
			return "", fmt.Errorf("not found")
		},
	})
	if err == nil {
		t.Fatal("expected missing prereq error")
	}
	s := out.String()
	if !strings.Contains(s, "Observe") {
		t.Fatalf("out=%s", s)
	}
	if !strings.Contains(s, "kind") || !strings.Contains(s, "not found") {
		t.Fatalf("out=%s", s)
	}
}

func TestRunGuide(t *testing.T) {
	var out bytes.Buffer
	err := Run(Options{
		Out: &out,
		LookPath: func(file string) (string, error) {
			return "/bin/" + file, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := out.String()
	if !strings.Contains(s, "make walkthrough") {
		t.Fatalf("missing walkthrough:\n%s", s)
	}
	if !strings.Contains(s, "init --ollama") {
		t.Fatalf("missing NL bridge:\n%s", s)
	}
	if !strings.Contains(s, "not the NL plan") {
		t.Fatalf("missing honesty:\n%s", s)
	}
}

func TestRunCheckOnlyOK(t *testing.T) {
	var out bytes.Buffer
	err := Run(Options{
		CheckOnly: true,
		Out:       &out,
		LookPath: func(file string) (string, error) {
			return "/usr/local/bin/" + file, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "All prerequisites found") {
		t.Fatalf("out=%s", out.String())
	}
}
