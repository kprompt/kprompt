package main

import "testing"

func TestResolveKubeContext(t *testing.T) {
	cases := []struct {
		name              string
		flag, root, file  string
		want              string
	}{
		{"flag wins", "flag-ctx", "root-ctx", "file-ctx", "flag-ctx"},
		{"root when flag empty", "", "root-ctx", "file-ctx", "root-ctx"},
		{"file when both empty", "", "", "file-ctx", "file-ctx"},
		{"all empty", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveKubeContext(tc.flag, tc.root, tc.file); got != tc.want {
				t.Fatalf("got %q want %q", got, tc.want)
			}
		})
	}
}
