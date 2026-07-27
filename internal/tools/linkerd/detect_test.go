package linkerd

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
			want: "Linkerd Server CRD not found (policy.linkerd.io/Server)",
		},
		{
			name: "installed with version",
			av: Availability{
				Installed: true,
				Group:     ServerGroup,
				Kind:      ServerKind,
				Versions:  []string{"v1beta3"},
			},
			want: "Linkerd Server CRD present (policy.linkerd.io/Server: v1beta3)",
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
	if !strings.Contains(strings.ToLower(hint), "linkerd") {
		t.Fatalf("InstallHint() = %q, want Linkerd reference", hint)
	}
}
