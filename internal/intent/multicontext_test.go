package intent

import "testing"

func TestParseMultiContexts(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"list deployments across staging and prod", []string{"staging", "prod"}},
		{"list pods across a, b, and c", []string{"a", "b", "c"}},
		{"explain api on staging and prod", []string{"staging", "prod"}},
		{"list deployments", nil},
		{"list across prod", nil}, // single — not multi
	}
	for _, tc := range cases {
		got := ParseMultiContexts(tc.in)
		if len(got) != len(tc.want) {
			t.Fatalf("%q: got %v want %v", tc.in, got, tc.want)
		}
		for i := range tc.want {
			if got[i] != tc.want[i] {
				t.Fatalf("%q: got %v want %v", tc.in, got, tc.want)
			}
		}
	}
}
