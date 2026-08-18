package crossplane

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
			want: "CompositeResourceDefinition CRD not found (apiextensions.crossplane.io)",
		},
		{
			name: "installed without versions",
			av:   Availability{Installed: true},
			want: "Crossplane XRD API present",
		},
		{
			name: "installed with version",
			av: Availability{
				Installed: true,
				Group:     XRDGroup,
				Kind:      XRDKind,
				Versions:  []string{"v1"},
			},
			want: "Crossplane XRD API present (apiextensions.crossplane.io/CompositeResourceDefinition: v1)",
		},
		{
			name: "installed with multiple versions",
			av: Availability{
				Installed: true,
				Group:     XRDGroup,
				Kind:      XRDKind,
				Versions:  []string{"v1", "v1beta1"},
			},
			want: "Crossplane XRD API present (apiextensions.crossplane.io/CompositeResourceDefinition: v1, v1beta1)",
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
	if !strings.Contains(strings.ToLower(hint), "crossplane") {
		t.Fatalf("InstallHint() = %q, want Crossplane reference", hint)
	}
}
