package ui

import (
	"reflect"
	"testing"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/optimize"
	toolgrafana "github.com/kprompt/kprompt/internal/tools/grafana"
)

func TestFormatPerformanceValue(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		unit  string
		want  string
	}{
		{name: "bytes below kib", value: 512, unit: "bytes", want: "512 bytes"},
		{name: "bytes kib", value: 2048, unit: "bytes", want: "2.00 KiB"},
		{name: "bytes mib", value: 1024 * 1024, unit: "bytes", want: "1.00 MiB"},
		{name: "bytes gib", value: 1024 * 1024 * 1024, unit: "bytes", want: "1.00 GiB"},
		{name: "seconds", value: 1.23456, unit: "seconds", want: "1.235s"},
		{name: "cores", value: 0.5, unit: "cores", want: "0.500 cores"},
		{name: "replicas", value: 3, unit: "replicas", want: "3"},
		{name: "other unit", value: 12.3456, unit: "ms", want: "12.346 ms"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatPerformanceValue(tt.value, tt.unit); got != tt.want {
				t.Fatalf("formatPerformanceValue(%v, %q) = %q, want %q", tt.value, tt.unit, got, tt.want)
			}
		})
	}
}

func TestQueryRowCells(t *testing.T) {
	tests := []struct {
		name    string
		headers []string
		row     cluster.Row
		want    []string
	}{
		{
			name:    "no headers includes split extras",
			headers: nil,
			row: cluster.Row{
				Namespace: "default",
				Name:      "api",
				Ready:     "1/1",
				Status:    "Running",
				Extra:     "10m\tteam-a",
			},
			want: []string{"default", "api", "1/1", "Running", "10m", "team-a"},
		},
		{
			name:    "maps known headers and extras",
			headers: []string{"NAME", "TYPE", "STATUS", "AGE", "OWNER"},
			row: cluster.Row{
				Name:   "svc-a",
				Ready:  "ClusterIP",
				Status: "10.0.0.1",
				Extra:  "3d\tplatform",
			},
			want: []string{"svc-a", "ClusterIP", "10.0.0.1", "3d", "platform"},
		},
		{
			name:    "age with single extra does not consume default slot twice",
			headers: []string{"AGE", "CUSTOM"},
			row: cluster.Row{
				Extra: "5m",
			},
			want: []string{"5m", "5m"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := queryRowCells(tt.headers, tt.row)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("queryRowCells(%v, %+v) = %#v, want %#v", tt.headers, tt.row, got, tt.want)
			}
		})
	}
}

func TestColorizeDiffLine(t *testing.T) {
	th := Theme{
		enabled: true,
		success: "<success>",
		danger:  "<danger>",
		muted:   "<muted>",
	}

	tests := []struct {
		name string
		line string
		want string
	}{
		{name: "plus", line: "+ replicas: 2", want: "<success>+ replicas: 2" + ansiReset},
		{name: "minus", line: "- replicas: 1", want: "<danger>- replicas: 1" + ansiReset},
		{name: "plain", line: " replicas: 2", want: "<muted> replicas: 2" + ansiReset},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := colorizeDiffLine(th, tt.line); got != tt.want {
				t.Fatalf("colorizeDiffLine(%q) = %q, want %q", tt.line, got, tt.want)
			}
		})
	}
}

func TestFormatWorkloadResources(t *testing.T) {
	tests := []struct {
		name string
		wl   optimize.Workload
		want string
	}{
		{
			name: "all requests and limits",
			wl: optimize.Workload{
				CPURequest:    "100m",
				MemoryRequest: "128Mi",
				CPULimit:      "500m",
				MemoryLimit:   "512Mi",
			},
			want: "cpuReq=100m memReq=128Mi cpuLim=500m memLim=512Mi",
		},
		{
			name: "missing requests flag",
			wl: optimize.Workload{
				MissingReq: true,
			},
			want: "no requests/limits",
		},
		{
			name: "empty workload",
			wl:   optimize.Workload{},
			want: "-",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatWorkloadResources(tt.wl); got != tt.want {
				t.Fatalf("formatWorkloadResources(%+v) = %q, want %q", tt.wl, got, tt.want)
			}
		})
	}
}

func TestGrafanaDatasourceLabel(t *testing.T) {
	tests := []struct {
		name   string
		source toolgrafana.Datasource
		want   string
	}{
		{
			name: "prefer name",
			source: toolgrafana.Datasource{
				Name: "main-prometheus",
				UID:  "uid-1",
				Type: "prometheus",
			},
			want: "main-prometheus",
		},
		{
			name: "fallback uid",
			source: toolgrafana.Datasource{
				UID:  "uid-2",
				Type: "loki",
			},
			want: "uid-2",
		},
		{
			name: "fallback type",
			source: toolgrafana.Datasource{
				Type: "tempo",
			},
			want: "tempo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := grafanaDatasourceLabel(tt.source); got != tt.want {
				t.Fatalf("grafanaDatasourceLabel(%+v) = %q, want %q", tt.source, got, tt.want)
			}
		})
	}
}
