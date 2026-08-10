package setup

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

type fakeRunner struct {
	goos     string
	paths    map[string]string
	ran      []string
	runErr   error
	tempDir  string
	lookMiss map[string]bool
}

func (f *fakeRunner) LookPath(file string) (string, error) {
	if f.lookMiss != nil && f.lookMiss[file] {
		return "", errors.New("not found")
	}
	if p, ok := f.paths[file]; ok {
		return p, nil
	}
	return "", errors.New("not found")
}

func (f *fakeRunner) Run(_ context.Context, name string, args []string, _ []string) error {
	f.ran = append(f.ran, name+" "+strings.Join(args, " "))
	if f.runErr != nil {
		return f.runErr
	}
	if name == "curl" {
		for i := 0; i+1 < len(args); i++ {
			if args[i] == "-o" {
				_ = os.WriteFile(args[i+1], []byte("#!/bin/sh\n"), 0o700)
			}
		}
	}
	if name == "brew" || strings.Contains(name, "get-helm") {
		if f.paths == nil {
			f.paths = map[string]string{}
		}
		delete(f.lookMiss, "helm")
		f.paths["helm"] = "/usr/local/bin/helm"
	}
	return nil
}

func (f *fakeRunner) TempDir() string {
	if f.tempDir != "" {
		return f.tempDir
	}
	return "/tmp"
}

func (f *fakeRunner) GOOS() string {
	if f.goos != "" {
		return f.goos
	}
	return "darwin"
}

func TestResolveHelmMethodBrewDarwin(t *testing.T) {
	r := &fakeRunner{goos: "darwin", paths: map[string]string{"brew": "/opt/homebrew/bin/brew"}, lookMiss: map[string]bool{"helm": true}}
	m, err := ResolveHelmMethod(r)
	if err != nil || m.ID != "brew" {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestResolveHelmMethodUnsupportedWindows(t *testing.T) {
	r := &fakeRunner{goos: "windows", lookMiss: map[string]bool{"helm": true, "brew": true}}
	m, err := ResolveHelmMethod(r)
	if err != nil || m.ID != "unsupported" {
		t.Fatalf("%+v %v", m, err)
	}
}

func TestApplyHostSkipsWhenOnPATH(t *testing.T) {
	plan := Plan{Steps: []Step{{
		ID: "helm", Component: "helm", Lane: LaneHost, Status: StatusNeeded,
	}}}
	r := &fakeRunner{paths: map[string]string{"helm": "/bin/helm"}}
	var buf bytes.Buffer
	rep, err := ApplyHost(context.Background(), plan, r, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 1 || rep.Applied[0].Status != "skipped" {
		t.Fatalf("%+v", rep)
	}
	if len(r.ran) != 0 {
		t.Fatalf("ran=%v", r.ran)
	}
}

func TestApplyHostBrewInstall(t *testing.T) {
	plan := Plan{Steps: []Step{{
		ID: "helm", Component: "helm", Lane: LaneHost, Status: StatusNeeded,
	}}}
	r := &fakeRunner{
		goos:     "darwin",
		paths:    map[string]string{"brew": "/opt/homebrew/bin/brew"},
		lookMiss: map[string]bool{"helm": true},
		tempDir:  t.TempDir(),
	}
	var buf bytes.Buffer
	rep, err := ApplyHost(context.Background(), plan, r, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 1 || rep.Applied[0].Status != "installed" {
		t.Fatalf("%+v", rep)
	}
	if len(r.ran) != 1 || !strings.Contains(r.ran[0], "brew install helm") {
		t.Fatalf("ran=%v", r.ran)
	}
}

func TestApplyHostLinuxGetHelm3(t *testing.T) {
	plan := Plan{Steps: []Step{{
		ID: "helm", Component: "helm", Lane: LaneHost, Status: StatusNeeded,
	}}}
	r := &fakeRunner{
		goos:     "linux",
		paths:    map[string]string{"curl": "/usr/bin/curl"},
		lookMiss: map[string]bool{"helm": true, "brew": true},
		tempDir:  t.TempDir(),
	}
	var buf bytes.Buffer
	rep, err := ApplyHost(context.Background(), plan, r, &buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 1 || rep.Applied[0].Status != "installed" || rep.Applied[0].Method != "get-helm-3" {
		t.Fatalf("%+v ran=%v", rep, r.ran)
	}
	if len(r.ran) < 2 {
		t.Fatalf("expected curl + installer runs, got %v", r.ran)
	}
	if !strings.Contains(r.ran[0], "curl -fsSL -o ") || !strings.Contains(r.ran[0], "https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3") {
		t.Fatalf("unexpected curl invocation: %v", r.ran[0])
	}
}

func TestApplyHostIgnoresClusterSteps(t *testing.T) {
	plan := Plan{Steps: []Step{
		{ID: "argo-workflows", Component: "argo-workflows", Lane: LaneCluster, Status: StatusNeeded},
		{ID: "prometheus", Component: "prometheus", Lane: LaneConfig, Status: StatusNeeded},
	}}
	r := &fakeRunner{}
	rep, err := ApplyHost(context.Background(), plan, r, &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Applied) != 0 {
		t.Fatalf("%+v", rep)
	}
}

func TestHostNeeded(t *testing.T) {
	plan := Plan{Steps: []Step{
		{Lane: LaneHost, Status: StatusNeeded, Component: "helm"},
		{Lane: LaneHost, Status: StatusReady, Component: "helm"},
		{Lane: LaneCluster, Status: StatusNeeded, Component: "argo"},
	}}
	got := HostNeeded(plan)
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

type captureRunner struct {
	fakeRunner
	lastCommand string
	lastArgs    []string
}

func (c *captureRunner) Run(ctx context.Context, name string, args []string, env []string) error {
	c.lastCommand = name
	c.lastArgs = append([]string(nil), args...)
	return c.fakeRunner.Run(ctx, name, args, env)
}

func TestHostSetupOnlyAllowsHardcodedURLs(t *testing.T) {
	method := getHelm3Method()
	if method.ID != "get-helm-3" {
		t.Fatalf("unexpected method ID: %s", method.ID)
	}
	r := &captureRunner{
		fakeRunner: fakeRunner{
			goos:     "linux",
			paths:    map[string]string{"curl": "/usr/bin/curl"},
			lookMiss: map[string]bool{"helm": true},
			tempDir:  t.TempDir(),
		},
	}
	script, _, _, err := method.Prepare(context.Background(), r)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(script, "get-helm-3") {
		t.Fatalf("unexpected script file path: %q", script)
	}

	// Verify the exact curl invocation token-by-token as requested in review
	if r.lastCommand != "curl" {
		t.Fatalf("expected command 'curl', got %q", r.lastCommand)
	}
	expectedArgs := []string{
		"-fsSL",
		"-o",
		script,
		"https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3",
	}
	if len(r.lastArgs) != len(expectedArgs) {
		t.Fatalf("expected exactly %d args, got %d: %v", len(expectedArgs), len(r.lastArgs), r.lastArgs)
	}
	for i, arg := range expectedArgs {
		if r.lastArgs[i] != arg {
			t.Fatalf("argument %d mismatch: got %q, want %q", i, r.lastArgs[i], arg)
		}
	}
}
