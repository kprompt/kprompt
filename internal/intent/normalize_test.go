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

func TestNormalizeWorkflowTrainModel(t *testing.T) {
	in := Intent{Kind: KindUnknown, Params: map[string]any{}}
	got := NormalizeVerb(in, `train a yolov11 model`)
	if got.Kind != KindWorkflow {
		t.Fatalf("kind=%s", got.Kind)
	}
	if got.Target.Name != "train-yolov11" {
		t.Fatalf("name=%s", got.Target.Name)
	}
	model, ok := got.Model()
	if !ok || model != "yolov11" {
		t.Fatalf("model=%q ok=%v", model, ok)
	}
}
