package argo

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/client-go/rest"
)

func TestRequireNotInstalled(t *testing.T) {
	err := Require(context.Background(), &rest.Config{Host: "https://127.0.0.1:1"})
	if err == nil {
		t.Skip("cluster unexpectedly reachable")
	}
	var ni NotInstalledError
	if !errors.As(err, &ni) {
		if !strings.Contains(err.Error(), "argo workflows detect") {
			t.Fatalf("unexpected error: %v", err)
		}
	}
}

func TestDetailLabelWithVersions(t *testing.T) {
	got := DetailLabel(Availability{
		Installed: true,
		Group:     WorkflowGroup,
		Kind:      WorkflowKind,
		Versions:  []string{"v1alpha1"},
	})
	if !strings.Contains(got, "v1alpha1") {
		t.Fatalf("detail=%q", got)
	}
}

func TestInstallHint(t *testing.T) {
	if InstallHint() == "" {
		t.Fatal("expected install hint")
	}
}

func TestNotInstalledError(t *testing.T) {
	err := NotInstalledError{Detail: "Workflow CRD not found"}
	if !strings.Contains(err.Error(), "Install Argo Workflows") {
		t.Fatalf("error=%q", err.Error())
	}
}
