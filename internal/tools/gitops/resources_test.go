package gitops

import (
	"context"
	"strings"
	"testing"
)

func TestListResourceDriftsSkipsNonArgoEngines(t *testing.T) {
	got, err := ListResourceDrifts(context.Background(), nil, AppStatus{Engine: "flux"})
	if err != nil || got != nil {
		t.Fatalf("got=%+v err=%v", got, err)
	}
}

func TestListResourceDriftsNilConfig(t *testing.T) {
	_, err := ListResourceDrifts(context.Background(), nil, AppStatus{Engine: "argocd"})
	if err == nil || !strings.Contains(err.Error(), "rest config is nil") {
		t.Fatalf("err=%v", err)
	}
}
