package safety

import (
	"strings"
	"testing"
)

func TestWipeDenyMessageFormat(t *testing.T) {
	msg := WipeDenyMessage("delete everything in the cluster")
	prefix := "🚨 Intent: destructive cluster operation\n🛡️ Safe execution: denied\n😅 "
	if !strings.HasPrefix(msg, prefix) {
		t.Fatalf("unexpected header:\n%s", msg)
	}
	if !strings.Contains(msg, WipeDenyRemediation) {
		t.Fatalf("missing remediation Next line:\n%s", msg)
	}
	punch := strings.TrimPrefix(msg, prefix)
	punch, _, _ = strings.Cut(punch, "\n\n")
	found := false
	for _, f := range wipeDenyFlavors {
		if punch == f {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("punchline not in flavor pack: %q", punch)
	}
}

func TestWipeDenyPunchlineStable(t *testing.T) {
	p := "wipe the cluster now"
	a := wipeDenyPunchline(p)
	b := wipeDenyPunchline(p)
	if a != b {
		t.Fatalf("same prompt must be stable: %q vs %q", a, b)
	}
}

func TestWipeDenyFlavorPackHasVariety(t *testing.T) {
	if len(wipeDenyFlavors) < 8 {
		t.Fatalf("want a real pack, got %d flavors", len(wipeDenyFlavors))
	}
	seen := map[string]bool{}
	prompts := []string{
		"delete everything in the cluster",
		"wipe the cluster now",
		"delete all pods",
		"destroy everything in the cluster",
		"delete the cluster",
		"remove my f*cking cluster",
		"delete all namespaces",
		"delete the namespace",
	}
	for _, p := range prompts {
		seen[wipeDenyPunchline(p)] = true
	}
	if len(seen) < 3 {
		t.Fatalf("expected variety across wipe prompts, got %d unique: %v", len(seen), seen)
	}
}

func TestCheckPromptWipeUsesFlavorPack(t *testing.T) {
	r := CheckPrompt("delete everything in the cluster")
	if !r.Denied {
		t.Fatal("expected deny")
	}
	want := WipeDenyMessage("delete everything in the cluster")
	if r.Message != want {
		t.Fatalf("message mismatch:\ngot:  %q\nwant: %q", r.Message, want)
	}
}
