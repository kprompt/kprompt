package intent

import "testing"

func TestLooksLikeSessionPrompt(t *testing.T) {
	yes := []string{
		"what did I do today",
		"session summary",
		"todays history",
	}
	for _, p := range yes {
		if !LooksLikeSessionPrompt(p) {
			t.Fatalf("expected match for %q", p)
		}
	}
	no := []string{
		"watch cluster",
		"remember that",
		"cluster vibe check",
	}
	for _, p := range no {
		if LooksLikeSessionPrompt(p) {
			t.Fatalf("unexpected match for %q", p)
		}
	}
}

func TestLooksLikeRememberPrompt(t *testing.T) {
	yes := []string{
		"remember that",
		"forget list",
	}
	for _, p := range yes {
		if !LooksLikeRememberPrompt(p) {
			t.Fatalf("expected match for %q", p)
		}
	}
	no := []string{
		"watch cluster",
		"todays history",
	}
	for _, p := range no {
		if LooksLikeRememberPrompt(p) {
			t.Fatalf("unexpected match for %q", p)
		}
	}
}

func TestLooksLikeWatchPrompt(t *testing.T) {
	yes := []string{
		"watch the cluster",
		"proactive assist",
		"watch payments",
	}
	for _, p := range yes {
		if !LooksLikeWatchPrompt(p) {
			t.Fatalf("expected match for %q", p)
		}
	}
	no := []string{
		"how's my cluster",
		"cluster vibe check",
		"roast my cluster",
	}
	for _, p := range no {
		if LooksLikeWatchPrompt(p) {
			t.Fatalf("unexpected match for %q", p)
		}
	}
}
