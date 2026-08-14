package tekton

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
			want: "PipelineRun CRD not found (tekton.dev/PipelineRun)",
		},
		{
			name: "installed without versions",
			av:   Availability{Installed: true},
			want: "PipelineRun CRD present",
		},
		{
			name: "installed with version",
			av: Availability{
				Installed: true,
				Group:     PipelineGroup,
				Kind:      PipelineRunKind,
				Versions:  []string{"v1"},
			},
			want: "PipelineRun CRD present (tekton.dev/PipelineRun: v1)",
		},
		{
			name: "installed with multiple versions",
			av: Availability{
				Installed: true,
				Group:     PipelineGroup,
				Kind:      PipelineRunKind,
				Versions:  []string{"v1", "v1beta1"},
			},
			want: "PipelineRun CRD present (tekton.dev/PipelineRun: v1, v1beta1)",
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
	if !strings.Contains(strings.ToLower(hint), "tekton") {
		t.Fatalf("InstallHint() = %q, want Tekton reference", hint)
	}
}
