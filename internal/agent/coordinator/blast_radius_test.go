package coordinator

import (
	"strings"
	"testing"
	"time"

	"github.com/kprompt/kprompt/internal/agent/handoff"
)

func recs(pairs ...[2]string) []Record {
	out := make([]Record, 0, len(pairs))
	now := time.Now().UTC()
	for _, p := range pairs {
		out = append(out, Record{
			Envelope: handoff.Envelope{FromNamespace: p[0], SuspectNamespace: p[1]},
			At:       now,
		})
	}
	return out
}

func TestHopRisk(t *testing.T) {
	cases := map[int]string{0: "low", 1: "low", 2: "medium", 4: "medium", 5: "high", 9: "high"}
	for count, want := range cases {
		if got := hopRisk(count); got != want {
			t.Errorf("hopRisk(%d)=%s want %s", count, got, want)
		}
	}
}

func TestBlastRadiusAllEdges(t *testing.T) {
	records := recs(
		[2]string{"a", "b"}, [2]string{"a", "b"}, [2]string{"a", "b"},
		[2]string{"a", "b"}, [2]string{"a", "b"}, // a->b x5 => high
		[2]string{"b", "c"}, [2]string{"b", "c"}, // b->c x2 => medium
		[2]string{"c", ""}, // no suspect
	)
	rep := BlastRadius(records, true, "", 0, false)
	if rep.Status != "degraded" {
		t.Fatalf("mesh off should be degraded, got %s", rep.Status)
	}
	if rep.HandoffCount != len(records) {
		t.Fatalf("handoffCount=%d want %d", rep.HandoffCount, len(records))
	}
	if len(rep.Hops) == 0 || rep.Hops[0].From != "a" || rep.Hops[0].To != "b" || rep.Hops[0].Risk != "high" {
		t.Fatalf("expected a->b high first, got %+v", rep.Hops)
	}
	if !contains(rep.Namespaces, "a") || !contains(rep.Namespaces, "c") {
		t.Fatalf("namespaces=%v", rep.Namespaces)
	}
}

func TestBlastRadiusMeshOK(t *testing.T) {
	rep := BlastRadius(recs([2]string{"a", "b"}), false, "", 3, true)
	if rep.Status != "ok" {
		t.Fatalf("mesh on should be ok, got %s", rep.Status)
	}
}

func TestBlastRadiusFocusFilter(t *testing.T) {
	records := recs(
		[2]string{"a", "b"},
		[2]string{"b", "c"},
		[2]string{"x", "y"}, // disconnected from focus a
	)
	rep := BlastRadius(records, false, "a", 1, false)
	// Within 1 hop of a: a->b only (b->c is 2 hops away).
	for _, h := range rep.Hops {
		if h.From == "x" || h.To == "y" {
			t.Fatalf("disconnected edge leaked: %+v", h)
		}
		if h.From == "b" && h.To == "c" {
			t.Fatalf("edge beyond maxHops leaked: %+v", h)
		}
	}
	if rep.FocusNamespace != "a" {
		t.Fatalf("focus=%q", rep.FocusNamespace)
	}
}

func TestFormatBlastRadius(t *testing.T) {
	rep := BlastRadius(recs([2]string{"a", "b"}, [2]string{"c", ""}), true, "", 3, false)
	out := FormatBlastRadius(rep)
	if !strings.Contains(out, "Coordinator blast-radius") {
		t.Fatalf("missing header: %s", out)
	}
	if !strings.Contains(out, "a -> b") {
		t.Fatalf("missing edge line: %s", out)
	}
	if !strings.Contains(out, "no suspect") {
		t.Fatalf("missing no-suspect line: %s", out)
	}
}

func TestServiceBlastRadius(t *testing.T) {
	s := New()
	s.MeshConfigured = true
	rep := s.BlastRadius("")
	if rep.Status != "ok" {
		t.Fatalf("service mesh on should be ok: %s", rep.Status)
	}
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
