package coach

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/tools"
)

func TestFormatNotReady(t *testing.T) {
	var buf bytes.Buffer
	st := Status{
		Version:       "0.0.0-test",
		KubeOK:        true,
		KubeDetail:    "kind-dev",
		LLMOK:         false,
		LLMDetail:     "no provider configured",
		ClusterOK:     true,
		ClusterDetail: "reachable",
	}
	if err := Format(&buf, st); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "not ready") {
		t.Fatalf("out=%s", out)
	}
	if !strings.Contains(out, "kprompt init --ollama") {
		t.Fatalf("missing init hint:\n%s", out)
	}
	if !strings.Contains(out, "kprompt demo") {
		t.Fatalf("missing demo hint:\n%s", out)
	}
	if !strings.Contains(out, `how's my cluster`) {
		t.Fatalf("missing roast hint:\n%s", out)
	}
}

func TestFormatReady(t *testing.T) {
	var buf bytes.Buffer
	st := Status{
		Version:       "1.0.0",
		KubeOK:        true,
		KubeDetail:    "ctx",
		LLMOK:         true,
		LLMDetail:     "ollama · llama3.2",
		ClusterOK:     true,
		ClusterDetail: "reachable",
	}
	if err := Format(&buf, st); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "ready for natural language") {
		t.Fatalf("out=%s", out)
	}
	if strings.Contains(out, "init --ollama") {
		t.Fatalf("ready coach should not push init:\n%s", out)
	}
	if !strings.Contains(out, `list pods`) {
		t.Fatalf("missing sample:\n%s", out)
	}
}

func TestGatherUnconfigured(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", dir+"/.kprompt")
	t.Setenv("KPROMPT_OPENAI_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	st, err := Gather(context.Background(), "test", Options{
		CurrentContext: func() (string, error) { return "kind-dev", nil },
		Detect: func(context.Context, tools.DetectOptions) (*tools.Registry, error) {
			return tools.NewRegistry([]tools.Result{
				{ID: tools.IDKubernetes, Name: "Kubernetes", Status: tools.StatusAvailable, Detail: "context: kind-dev"},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if st.LLMOK {
		t.Fatalf("expected llm fail: %+v", st)
	}
	if !st.KubeOK || !st.ClusterOK {
		t.Fatalf("kube/cluster: %+v", st)
	}
	if st.Ready() {
		t.Fatal("should not be ready")
	}
}

func TestGatherOllamaReady(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("KPROMPT_HOME", dir+"/.kprompt")

	if _, err := config.SetField("provider", "ollama"); err != nil {
		t.Fatal(err)
	}
	if _, err := config.SetField("model", "llama3.2"); err != nil {
		t.Fatal(err)
	}

	st, err := Gather(context.Background(), "test", Options{
		CurrentContext: func() (string, error) { return "c", nil },
		Detect: func(context.Context, tools.DetectOptions) (*tools.Registry, error) {
			return tools.NewRegistry([]tools.Result{
				{ID: tools.IDKubernetes, Name: "Kubernetes", Status: tools.StatusAvailable, Detail: "ok"},
			}), nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !st.Ready() {
		t.Fatalf("want ready: %+v", st)
	}
}

func TestFormatBrief(t *testing.T) {
	var buf bytes.Buffer
	if err := FormatBrief(&buf, Status{}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "init --ollama") {
		t.Fatalf("%s", buf.String())
	}
}
