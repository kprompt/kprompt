package gitops

import (
	"context"
	"strings"
	"testing"
)

func TestListResourceDriftsUnknownEngine(t *testing.T) {
	got, err := ListResourceDrifts(context.Background(), nil, AppStatus{Engine: "other"})
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestListResourceDriftsNilConfigArgo(t *testing.T) {
	_, err := ListResourceDrifts(context.Background(), nil, AppStatus{Engine: "argocd"})
	if err == nil || !strings.Contains(err.Error(), "rest config is nil") {
		t.Fatalf("err=%v", err)
	}
}

func TestListResourceDriftsNilConfigFlux(t *testing.T) {
	_, err := ListResourceDrifts(context.Background(), nil, AppStatus{Engine: "flux", Kind: FluxKind})
	if err == nil || !strings.Contains(err.Error(), "rest config is nil") {
		t.Fatalf("err=%v", err)
	}
}

func TestListResourceDriftsFluxHelmReleaseDegrades(t *testing.T) {
	_, err := ListResourceDrifts(context.Background(), nil, AppStatus{Engine: "flux", Kind: "HelmRelease"})
	if err == nil || !strings.Contains(err.Error(), "inventory unavailable") {
		t.Fatalf("err=%v", err)
	}
}
