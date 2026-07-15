package executor

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/planner"
)

func TestDeleteDeployment(t *testing.T) {
	client := fake.NewSimpleClientset(&appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: "redis", Namespace: "demo"},
	})
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op: planner.OpDelete,
			Object: planner.ObjectRef{
				Kind: "Deployment", Name: "redis", Namespace: "demo",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.AppsV1().Deployments("demo").Get(context.Background(), "redis", metav1.GetOptions{})
	if err == nil {
		t.Fatal("expected deployment deleted")
	}
}
