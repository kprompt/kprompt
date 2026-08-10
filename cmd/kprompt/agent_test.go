package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/kprompt/kprompt/internal/agent/correlate"
	"github.com/kprompt/kprompt/internal/agent/ctxbuild"
	"github.com/kprompt/kprompt/internal/agent/autopilot"
	"github.com/kprompt/kprompt/internal/incident"
)

// hasSub reports whether cmd has a direct subcommand whose Use starts with name.
func hasSub(cmd *cobra.Command, name string) bool {
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func findSub(t *testing.T, cmd *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range cmd.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("subcommand %q not found under %q", name, cmd.Name())
	return nil
}

func TestNewAgentCmdTree(t *testing.T) {
	root := newAgentCmd()
	if root.Name() != "agent" {
		t.Fatalf("root name=%q", root.Name())
	}
	for _, sub := range []string{"run", "list", "status", "operator", "coordinator", "autopilot", "memory", "patterns", "proposals", "graph"} {
		if !hasSub(root, sub) {
			t.Errorf("missing agent subcommand %q", sub)
		}
	}
	coord := findSub(t, root, "coordinator")
	for _, sub := range []string{"knowledge", "blast-radius", "recent", "outcomes"} {
		if !hasSub(coord, sub) {
			t.Errorf("missing coordinator subcommand %q", sub)
		}
	}
	if !hasSub(findSub(t, root, "autopilot"), "apply-proposal") {
		t.Error("missing autopilot apply-proposal")
	}
	mem := findSub(t, root, "memory")
	for _, sub := range []string{"list", "set", "discover", "export"} {
		if !hasSub(mem, sub) {
			t.Errorf("missing memory subcommand %q", sub)
		}
	}
	prop := findSub(t, root, "proposals")
	for _, sub := range []string{"list", "show", "apply"} {
		if !hasSub(prop, sub) {
			t.Errorf("missing proposals subcommand %q", sub)
		}
	}
	if !hasSub(findSub(t, root, "patterns"), "list") {
		t.Error("missing patterns list")
	}
}

func execAgent(args ...string) (string, error) {
	root := &cobra.Command{Use: "kprompt"}
	root.AddCommand(newAgentCmd())
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestAgentRunEErrorPaths(t *testing.T) {
	cases := [][]string{
		{"agent", "autopilot", "apply-proposal", "--file", "/nope"}, // no --approve
		{"agent", "proposals", "apply", "-n", "ns", "--id", "x"},    // no --approve
		{"agent", "memory", "export"},                               // no -n / --fleet
	}
	for _, args := range cases {
		if _, err := execAgent(args...); err == nil {
			t.Errorf("expected error for %v", args)
		}
	}
}

func TestAgentRunConnectError(t *testing.T) {
	// Force kubeconfig to an empty file so cluster.Connect fails cleanly (no network),
	// after the RunE flag-consequence logic has executed.
	dir := t.TempDir()
	kubeconfig := dir + "/empty-kubeconfig"
	if err := os.WriteFile(kubeconfig, []byte("apiVersion: v1\nkind: Config\nclusters: []\ncontexts: []\nusers: []\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KUBECONFIG", kubeconfig)

	if _, err := execAgent("agent", "run", "-n", "demo", "--analyze", "--heuristic", "--memory", "--patterns", "--slack-ask"); err == nil {
		t.Fatal("expected cluster connect error with empty kubeconfig")
	}
}

func TestOpenStores(t *testing.T) {
	dir := t.TempDir()

	// memory: file ok, empty -> file default, configmap without client -> err, bogus -> err.
	if _, err := openMemoryStore("file", dir, "ns", false, nil); err != nil {
		t.Errorf("memory file: %v", err)
	}
	if _, err := openMemoryStore("", dir, "ns", false, nil); err != nil {
		t.Errorf("memory default: %v", err)
	}
	if _, err := openMemoryStore("configmap", dir, "ns", false, nil); err == nil {
		t.Error("memory configmap without client should error")
	}
	if _, err := openMemoryStore("bogus", dir, "ns", false, nil); err == nil {
		t.Error("memory bogus should error")
	}

	// incidents: "" is an error (no default), file ok, configmap err.
	if _, err := openIncidentsStore("file", dir, "ns", false, nil); err != nil {
		t.Errorf("incidents file: %v", err)
	}
	if _, err := openIncidentsStore("", dir, "ns", false, nil); err == nil {
		t.Error("incidents empty backend should error")
	}
	if _, err := openIncidentsStore("configmap", dir, "ns", false, nil); err == nil {
		t.Error("incidents configmap without client should error")
	}

	// proposals: file ok, configmap err, bogus err.
	if _, err := openProposalsStore("file", dir, "ns", false, nil); err != nil {
		t.Errorf("proposals file: %v", err)
	}
	if _, err := openProposalsStore("configmap", dir, "ns", false, nil); err == nil {
		t.Error("proposals configmap without client should error")
	}
	if _, err := openProposalsStore("bogus", dir, "ns", false, nil); err == nil {
		t.Error("proposals bogus should error")
	}

	// patterns: file ok, configmap err, bogus err.
	if _, err := openPatternsStore("file", dir, "ns", false, nil); err != nil {
		t.Errorf("patterns file: %v", err)
	}
	if _, err := openPatternsStore("configmap", dir, "ns", false, nil); err == nil {
		t.Error("patterns configmap without client should error")
	}
	if _, err := openPatternsStore("bogus", dir, "ns", false, nil); err == nil {
		t.Error("patterns bogus should error")
	}
}

func TestConnectOptionalNoClient(t *testing.T) {
	clients, err := connectOptional("", false, false)
	if err != nil || clients != nil {
		t.Fatalf("connectOptional(needClient=false) = %v, %v", clients, err)
	}
}

func TestFetchCoordinatorJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/bad" {
			http.Error(w, "nope", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	if _, err := fetchCoordinatorJSON(context.Background(), "", "/x"); err == nil {
		t.Error("empty url should error")
	}
	body, err := fetchCoordinatorJSON(context.Background(), srv.URL, "/v1/knowledge")
	if err != nil || !strings.Contains(string(body), "ok") {
		t.Fatalf("fetch: %v body=%s", err, body)
	}
	if _, err := fetchCoordinatorJSON(context.Background(), srv.URL, "/bad"); err == nil {
		t.Error("non-200 should error")
	}
}

func TestCoordinatorClientSubcommands(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/knowledge":
			_, _ = w.Write([]byte(`{"handoffCount":1,"durable":true,"edges":[{"from":"a","suspect":"b","count":1}]}`))
		case "/v1/blast-radius":
			_, _ = w.Write([]byte(`{"status":"degraded","durable":false,"maxHops":3,"handoffCount":1,"hops":[{"from":"a","to":"b","count":1,"risk":"low"}]}`))
		case "/v1/outcomes":
			_, _ = w.Write([]byte(`{"total":2,"durable":true,"byResult":{"apply_success":2}}`))
		case "/v1/recent":
			_, _ = w.Write([]byte(`[]`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	cases := [][]string{
		{"agent", "coordinator", "knowledge", "--url", srv.URL},
		{"agent", "coordinator", "knowledge", "--url", srv.URL, "--json"},
		{"agent", "coordinator", "blast-radius", "--url", srv.URL, "-n", "a"},
		{"agent", "coordinator", "blast-radius", "--url", srv.URL, "--json"},
		{"agent", "coordinator", "outcomes", "--url", srv.URL},
		{"agent", "coordinator", "outcomes", "--url", srv.URL, "--json"},
		{"agent", "coordinator", "recent", "--url", srv.URL},
	}
	for _, args := range cases {
		if out, err := execAgent(args...); err != nil {
			t.Errorf("%v: unexpected error %v (out=%s)", args, err, out)
		}
	}
}

func TestStampIncidentApplyOutcome(t *testing.T) {
	// nil args must be a no-op (no panic).
	stampIncidentApplyOutcome(nil, nil, nil)

	builder := correlate.NewBuilder(correlate.Options{Namespace: "ns"})
	agentCtx := &ctxbuild.AgentContext{Incident: incident.Incident{ID: "inc-1"}}
	prop := &autopilot.Proposal{
		Outcome:      "apply_success",
		VerifyStatus: "ok",
		ActionID:     autopilot.ActionRestartDeployment,
	}
	stampIncidentApplyOutcome(builder, agentCtx, prop)
	if agentCtx.Incident.LastApplyOutcome != "apply_success" {
		t.Fatalf("expected outcome stamped, got %q", agentCtx.Incident.LastApplyOutcome)
	}
	if agentCtx.Incident.LastActionID != autopilot.ActionRestartDeployment {
		t.Fatalf("expected action stamped, got %q", agentCtx.Incident.LastActionID)
	}
}