package istio

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
			want: "VirtualService CRD not found (networking.istio.io/VirtualService)",
		},
		{
			name: "installed without versions",
			av:   Availability{Installed: true},
			want: "VirtualService CRD present",
		},
		{
			name: "installed with version",
			av: Availability{
				Installed: true,
				Group:     VirtualServiceGroup,
				Kind:      VirtualServiceKind,
				Versions:  []string{"v1beta1"},
			},
			want: "VirtualService CRD present (networking.istio.io/VirtualService: v1beta1)",
		},
		{
			name: "installed with multiple versions",
			av: Availability{
				Installed: true,
				Group:     VirtualServiceGroup,
				Kind:      VirtualServiceKind,
				Versions:  []string{"v1beta1", "v1alpha3"},
			},
			want: "VirtualService CRD present (networking.istio.io/VirtualService: v1beta1, v1alpha3)",
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
	if !strings.Contains(strings.ToLower(hint), "istio") {
		t.Fatalf("InstallHint() = %q, want Istio reference", hint)
	}
}
