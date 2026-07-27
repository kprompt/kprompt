package gateway

import (
	"strings"
	"testing"
)

func TestDetailLabel(t *testing.T) {
	tests := []struct {
		name string
		av   Availability
		want string
	}{
		{
			name: "not installed",
			want: "Gateway CRD not found (gateway.networking.k8s.io/Gateway)",
		},
		{
			name: "installed with version",
			av: Availability{
				Installed: true,
				Group:     GatewayGroup,
				Kind:      GatewayKind,
				Versions:  []string{"v1"},
			},
			want: "Gateway API Gateway CRD present (gateway.networking.k8s.io/Gateway: v1)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := DetailLabel(tt.av); got != tt.want {
				t.Fatalf("DetailLabel() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestInstallHint(t *testing.T) {
	hint := InstallHint()
	if hint == "" {
		t.Fatal("expected install hint")
	}
	if !strings.Contains(strings.ToLower(hint), "gateway api") {
		t.Fatalf("InstallHint() = %q, want Gateway API reference", hint)
	}
}
