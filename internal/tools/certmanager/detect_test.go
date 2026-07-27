package certmanager

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
			want: "Certificate CRD not found (cert-manager.io/Certificate)",
		},
		{
			name: "installed with version",
			av: Availability{
				Installed: true,
				Group:     CertificateGroup,
				Kind:      CertificateKind,
				Versions:  []string{"v1"},
			},
			want: "cert-manager Certificate CRD present (cert-manager.io/Certificate: v1)",
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
	if !strings.Contains(strings.ToLower(hint), "cert-manager") {
		t.Fatalf("InstallHint() = %q, want cert-manager reference", hint)
	}
}
