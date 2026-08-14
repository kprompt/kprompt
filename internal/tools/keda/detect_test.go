package keda

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
			want: "ScaledObject CRD not found (keda.sh/ScaledObject)",
		},
		{
			name: "installed without versions",
			av:   Availability{Installed: true},
			want: "ScaledObject CRD present",
		},
		{
			name: "installed with version",
			av: Availability{
				Installed: true,
				Group:     ScaledObjectGroup,
				Kind:      ScaledObjectKind,
				Versions:  []string{"v1alpha1"},
			},
			want: "ScaledObject CRD present (keda.sh/ScaledObject: v1alpha1)",
		},
		{
			name: "installed with multiple versions",
			av: Availability{
				Installed: true,
				Group:     ScaledObjectGroup,
				Kind:      ScaledObjectKind,
				Versions:  []string{"v1alpha1", "v1beta1"},
			},
			want: "ScaledObject CRD present (keda.sh/ScaledObject: v1alpha1, v1beta1)",
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
	if !strings.Contains(strings.ToLower(hint), "keda") {
		t.Fatalf("InstallHint() = %q, want KEDA reference", hint)
	}
}
