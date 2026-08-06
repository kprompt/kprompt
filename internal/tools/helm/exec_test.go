package helm

import (
	"context"
	"strings"
	"testing"
)

func TestRunCaptureRejectsNonHelmEntryPoint(t *testing.T) {
	_, err := RunCapture(context.Background(), []string{"helm install redis bitnami/redis"})
	if err == nil || !strings.Contains(err.Error(), "invalid helm command") {
		t.Fatalf("err=%v", err)
	}
}
