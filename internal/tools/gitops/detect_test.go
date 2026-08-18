package gitops

import (
	"strings"
	"testing"
)

func TestDetailLabelCases(t *testing.T) {
	tests := []struct {
		name string
		av   Availability
		want string
	}{
		{
			name: "not installed",
			want: "Flux Kustomization / Argo CD Application CRDs not found",
		},
		{
			name: "flux installed",
			av: Availability{
				Installed: true,
				Flux:      true,
			},
			want: "Flux Kustomization present",
		},
		{
			name: "argo cd installed",
			av: Availability{
				Installed: true,
				ArgoCD:    true,
			},
			want: "Argo CD Application present",
		},
		{
			name: "flux and argo cd installed",
			av: Availability{
				Installed: true,
				Flux:      true,
				ArgoCD:    true,
			},
			want: "Flux Kustomization + Argo CD Application present",
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
	lowerHint := strings.ToLower(hint)
	for _, product := range []string{"flux", "argo cd"} {
		if !strings.Contains(lowerHint, product) {
			t.Fatalf("InstallHint() = %q, want %s reference", hint, product)
		}
	}
}
