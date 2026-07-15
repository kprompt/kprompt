package cluster

import (
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestDiagnosePodOOMAndCrash(t *testing.T) {
	pod := corev1.Pod{}
	pod.Name = "api"
	pod.Status.ContainerStatuses = []corev1.ContainerStatus{{
		Name: "app",
		State: corev1.ContainerState{
			Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff", Message: "back-off"},
		},
		LastTerminationState: corev1.ContainerState{
			Terminated: &corev1.ContainerStateTerminated{Reason: "OOMKilled", ExitCode: 137},
		},
		RestartCount: 5,
	}}
	findings := diagnosePod(pod)
	codes := map[string]bool{}
	for _, f := range findings {
		codes[f.Code] = true
	}
	for _, want := range []string{"CrashLoopBackOff", "OOMKilled", "Restarts"} {
		if !codes[want] {
			t.Fatalf("missing finding %s in %+v", want, findings)
		}
	}
}

func TestSummarizeHealthy(t *testing.T) {
	s := summarize(ExplainReport{Kind: "Deployment", Target: "ok", Status: "ready 1/1"})
	if !strings.Contains(s, "healthy") {
		t.Fatalf("summary=%q", s)
	}
}
