//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestLogsAndDescribeOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)
	ensureDeployment(t, ctx, client, 1)
	waitDemoPodRunning(t, ctx, client)

	var descOut bytes.Buffer
	err := pipeline.RunWith(ctx, config.Resolved{
		Namespace: ns,
		Prompt:    "describe demo",
	}, &descOut, pipeline.Deps{
		Provider: llm.DescribeStub(deployName, ns, "Deployment"),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("describe: %v\n%s", err, descOut.String())
	}
	if !strings.Contains(descOut.String(), "Deployment/"+deployName) {
		t.Fatalf("describe output: %s", descOut.String())
	}
	t.Log(descOut.String())

	var logsOut bytes.Buffer
	err = pipeline.RunWith(ctx, config.Resolved{
		Namespace: ns,
		Prompt:    "show logs for demo",
	}, &logsOut, pipeline.Deps{
		Provider: llm.LogsStub(deployName, ns, "Deployment", 20, ""),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("logs: %v\n%s", err, logsOut.String())
	}
	if !strings.Contains(logsOut.String(), "Logs: Pod/") {
		t.Fatalf("logs output: %s", logsOut.String())
	}
	t.Log(logsOut.String())
}

func waitDemoPodRunning(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Minute)
	for time.Now().Before(deadline) {
		list, err := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{
			LabelSelector: "app=" + deployName,
		})
		if err == nil {
			for _, pod := range list.Items {
				if pod.Status.Phase == corev1.PodRunning {
					return
				}
			}
			// deploy may use different selector if created differently
			list2, err2 := client.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
			if err2 == nil {
				for _, pod := range list2.Items {
					if strings.HasPrefix(pod.Name, deployName) && pod.Status.Phase == corev1.PodRunning {
						return
					}
				}
			}
		}
		select {
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		case <-time.After(500 * time.Millisecond):
		}
	}
	t.Fatal("timed out waiting for demo pod Running")
}
