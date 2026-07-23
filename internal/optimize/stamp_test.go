package optimize

import "testing"

func TestStampClusterContext(t *testing.T) {
	r := BuildScaffold(Request{})
	StampClusterContext(&r, "prod-gke")
	if r.ClusterContext != "prod-gke" {
		t.Fatalf("%q", r.ClusterContext)
	}
	if len(r.Findings) == 0 || r.Findings[0].ClusterContext != "prod-gke" {
		t.Fatalf("%+v", r.Findings)
	}
}
