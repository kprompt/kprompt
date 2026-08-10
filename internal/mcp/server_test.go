package mcp

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// roundTrip feeds one JSON-RPC request line through the server and returns the
// first response line.
func roundTrip(t *testing.T, line string) response {
	t.Helper()
	srv := NewServer(strings.NewReader(line+"\n"), &discardWriter{}, "test")
	var out capturingWriter
	srv.out = &out
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	raw := strings.TrimSpace(out.b.String())
	if raw == "" {
		t.Fatalf("no response for %q", line)
	}
	var resp response
	if err := json.Unmarshal([]byte(strings.SplitN(raw, "\n", 2)[0]), &resp); err != nil {
		t.Fatalf("unmarshal response: %v (raw=%q)", err, raw)
	}
	return resp
}

func TestInitializeReportsServerInfo(t *testing.T) {
	resp := roundTrip(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18"}}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]any)
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != serverName {
		t.Fatalf("serverInfo.name = %v, want %s", info["name"], serverName)
	}
}

func TestToolsListAdvertisesReadOnlyTools(t *testing.T) {
	resp := roundTrip(t, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]any)
	tools, _ := result["tools"].([]any)
	got := map[string]bool{}
	for _, ti := range tools {
		m, _ := ti.(map[string]any)
		got[m["name"].(string)] = true
	}
	for _, want := range []string{
		"kprompt.read", "kprompt.tools", "kprompt.doctor",
		"kprompt.investigate", "kprompt.why", "kprompt.timeline", "kprompt.impact",
		"kprompt.plan",
	} {
		if !got[want] {
			t.Fatalf("tools/list missing %q; got %v", want, got)
		}
	}
}

// A wipe-class prompt must return a denied PlanResult over MCP and never apply —
// the hard-deny invariant reaches the MCP surface (ADR-0024 §3).
func TestPlanHardDenyReturnsDeniedAndNeverApplies(t *testing.T) {
	t.Setenv("KPROMPT_HOME", t.TempDir())
	resp := roundTrip(t, `{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"kprompt.plan","arguments":{"prompt":"delete everything in the cluster"}}}`)
	if resp.Error != nil {
		t.Fatalf("unexpected transport error: %+v", resp.Error)
	}
	result, _ := resp.Result.(map[string]any)
	if isErr, _ := result["isError"].(bool); isErr {
		t.Fatalf("hard-deny should be a normal result, not isError: %v", result)
	}
	content, _ := result["content"].([]any)
	if len(content) == 0 {
		t.Fatalf("no content in result: %v", result)
	}
	text, _ := content[0].(map[string]any)["text"].(string)
	if !strings.Contains(text, `"denied":true`) {
		t.Fatalf("expected denied PlanResult, got: %s", text)
	}
	if strings.Contains(text, `"applied":true`) {
		t.Fatalf("hard-deny must not apply: %s", text)
	}
}

func TestUnknownMethodReturnsError(t *testing.T) {
	resp := roundTrip(t, `{"jsonrpc":"2.0","id":3,"method":"nope"}`)
	if resp.Error == nil || resp.Error.Code != codeMethodNotFound {
		t.Fatalf("want method-not-found error, got %+v", resp.Error)
	}
}

func TestNotificationProducesNoResponse(t *testing.T) {
	srv := NewServer(strings.NewReader(`{"jsonrpc":"2.0","method":"notifications/initialized"}`+"\n"), &discardWriter{}, "test")
	var out capturingWriter
	srv.out = &out
	if err := srv.Serve(context.Background()); err != nil {
		t.Fatalf("Serve: %v", err)
	}
	if strings.TrimSpace(out.b.String()) != "" {
		t.Fatalf("notification produced a response: %q", out.b.String())
	}
}

// denyApply must never approve an apply — the core MCP safety invariant.
func TestDenyApplyNeverApproves(t *testing.T) {
	ok, err := denyApply(&discardWriter{})
	if err != nil || ok {
		t.Fatalf("denyApply = (%v,%v), want (false,nil)", ok, err)
	}
}

type capturingWriter struct{ b strings.Builder }

func (c *capturingWriter) Write(p []byte) (int, error) { return c.b.Write(p) }

type discardWriter struct{}

func (discardWriter) Write(p []byte) (int, error) { return len(p), nil }
