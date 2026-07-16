package intent

import "testing"

func TestNormalizeVerbInstall(t *testing.T) {
	in := Intent{Kind: KindDeploy, Target: Target{Name: "redis"}}
	got := NormalizeVerb(in, `install redis in staging`)
	if got.Kind != KindInstall {
		t.Fatalf("kind=%s", got.Kind)
	}
}

func TestNormalizeVerbDeployUnchanged(t *testing.T) {
	in := Intent{Kind: KindDeploy, Target: Target{Name: "redis"}}
	got := NormalizeVerb(in, `deploy redis`)
	if got.Kind != KindDeploy {
		t.Fatalf("kind=%s", got.Kind)
	}
}
