//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/kprompt/kprompt/internal/config"
	"github.com/kprompt/kprompt/internal/llm"
	"github.com/kprompt/kprompt/internal/pipeline"
)

func TestDeleteDeploymentOnKind(t *testing.T) {
	requireKind(t)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	ensureKindCluster(t, ctx)
	kubeconfig := exportKubeconfig(t, ctx)
	t.Setenv("KUBECONFIG", kubeconfig)

	client := clientFromKubeconfig(t, kubeconfig)
	ensureNamespace(t, ctx, client)

	name := "kprompt-delete-me"
	createNamedDeployment(t, ctx, client, name)
	defer func() {
		_ = client.AppsV1().Deployments(ns).Delete(ctx, name, metav1.DeleteOptions{})
	}()

	var out bytes.Buffer
	err := pipeline.RunWith(ctx, config.Resolved{
		Namespace: ns,
		Approve:   true,
		Prompt:    "delete deployment " + name,
	}, &out, pipeline.Deps{
		Provider: llm.DeleteStub(name, ns, "Deployment"),
		Client:   client,
	})
	if err != nil {
		t.Fatalf("pipeline: %v\n%s", err, out.String())
	}
	t.Log(out.String())

	_, err = client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
	if err == nil || !apierrors.IsNotFound(err) {
		t.Fatalf("expected NotFound after delete, got %v", err)
	}
}

func createNamedDeployment(t *testing.T, ctx context.Context, client kubernetes.Interface, name string) {
	t.Helper()
	labels := map[string]string{"app": name}
	replicas := int32(1)
	_, err := client.AppsV1().Deployments(ns).Create(ctx, &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "nginx",
						Image: "nginx:1.27-alpine",
					}},
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}
