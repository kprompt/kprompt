package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/tools"
)

func TestPrintToolsTableNextSteps(t *testing.T) {
	reg := tools.NewRegistry([]tools.Result{
		{
			ID: tools.IDKubernetes, Name: "Kubernetes",
			Status: tools.StatusAvailable, Detail: "context: kind-dev",
		},
		{
			ID: tools.IDHelm, Name: "Helm",
			Status: tools.StatusUnavailable, Detail: "not on PATH",
			Hint: tools.MissingHint(tools.IDHelm),
		},
		{
			ID: tools.IDGitOps, Name: "GitOps (Flux/Argo CD)",
			Status: tools.StatusUnavailable, Detail: "CRDs not found",
			Hint: tools.MissingHint(tools.IDGitOps),
		},
		{
			ID: tools.IDPrometheus, Name: "Prometheus",
			Status: tools.StatusUnavailable, Detail: "URL not set",
			Hint: tools.MissingHint(tools.IDPrometheus),
		},
	})

	var buf bytes.Buffer
	if err := printToolsTable(&buf, reg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "TOOL") || !strings.Contains(out, "unavailable") {
		t.Fatalf("missing table:\n%s", out)
	}
	if !strings.Contains(out, "Next steps (unavailable):") {
		t.Fatalf("missing Next steps:\n%s", out)
	}
	if !strings.Contains(out, "  - Helm:") || !strings.Contains(out, "kprompt setup") {
		t.Fatalf("missing Helm hint:\n%s", out)
	}
	if !strings.Contains(out, "  - GitOps (Flux/Argo CD):") || !strings.Contains(out, "flux bootstrap") {
		t.Fatalf("missing GitOps hint:\n%s", out)
	}
	if !strings.Contains(out, "Try: kprompt setup") {
		t.Fatalf("missing setup footer:\n%s", out)
	}
	if !strings.Contains(out, "KPROMPT_PROMETHEUS_URL") {
		t.Fatalf("missing URL footer:\n%s", out)
	}
}

func TestPrintToolsTableSkipsAvailableHints(t *testing.T) {
	reg := tools.NewRegistry([]tools.Result{
		{
			ID: tools.IDKubernetes, Name: "Kubernetes",
			Status: tools.StatusAvailable, Detail: "ok",
			Hint: "should not print",
		},
		{
			ID: tools.IDHelm, Name: "Helm",
			Status: tools.StatusAvailable, Detail: "v3",
			Hint: "should not print either",
		},
	})

	var buf bytes.Buffer
	if err := printToolsTable(&buf, reg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if strings.Contains(out, "Next steps") {
		t.Fatalf("unexpected Next steps:\n%s", out)
	}
	if strings.Contains(out, "should not print") {
		t.Fatalf("printed available hint:\n%s", out)
	}
	if strings.Contains(out, "Try: kprompt setup") {
		t.Fatalf("unexpected setup footer:\n%s", out)
	}
}

func TestPrintToolsTableManualOnlyNoSetupFooter(t *testing.T) {
	reg := tools.NewRegistry([]tools.Result{
		{
			ID: tools.IDGitOps, Name: "GitOps (Flux/Argo CD)",
			Status: tools.StatusUnavailable, Detail: "missing",
			Hint: tools.MissingHint(tools.IDGitOps),
		},
	})

	var buf bytes.Buffer
	if err := printToolsTable(&buf, reg); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "Next steps (unavailable):") {
		t.Fatalf("missing Next steps:\n%s", out)
	}
	if strings.Contains(out, "Try: kprompt setup") {
		t.Fatalf("setup footer should not appear for GitOps-only gap:\n%s", out)
	}
	if strings.Contains(out, "KPROMPT_PROMETHEUS_URL") {
		t.Fatalf("URL footer should not appear:\n%s", out)
	}
}
