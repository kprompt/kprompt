package executor

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/intent"
	"github.com/kprompt/kprompt/internal/planner"
)

func TestRollbackToPreviousRevision(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api",
			Namespace: "default",
			UID:       "dep-uid",
			Annotations: map[string]string{
				revisionAnnotation: "2",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api", appsv1.DefaultDeploymentUniqueLabelKey: "new"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx:new"}},
				},
			},
		},
	}
	rs1 := replicaSet("api", "default", "dep-uid", "1", "nginx:old", "oldhash")
	rs2 := replicaSet("api", "default", "dep-uid", "2", "nginx:new", "new")

	client := fake.NewSimpleClientset(dep, rs1, rs2)
	runner := &Runner{Client: client}
	err := runner.Apply(context.Background(), planner.ExecutionPlan{
		Intent: intent.Intent{Kind: intent.KindRollback},
		Actions: []planner.Action{{
			Op: planner.OpRollback,
			Object: planner.ObjectRef{
				Kind:      "Deployment",
				Name:      "api",
				Namespace: "default",
			},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if got.Spec.Template.Spec.Containers[0].Image != "nginx:old" {
		t.Fatalf("image=%s", got.Spec.Template.Spec.Containers[0].Image)
	}
	if _, ok := got.Spec.Template.Labels[appsv1.DefaultDeploymentUniqueLabelKey]; ok {
		t.Fatal("pod-template-hash should be cleared so a new RS can form")
	}
}

func TestRollbackToSpecificRevision(t *testing.T) {
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "api",
			Namespace: "default",
			UID:       "dep-uid",
			Annotations: map[string]string{
				revisionAnnotation: "3",
			},
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": "api"}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: "nginx:v3"}},
				},
			},
		},
	}
	client := fake.NewSimpleClientset(
		dep,
		replicaSet("api", "default", "dep-uid", "1", "nginx:v1", "h1"),
		replicaSet("api", "default", "dep-uid", "2", "nginx:v2", "h2"),
		replicaSet("api", "default", "dep-uid", "3", "nginx:v3", "h3"),
	)
	rev := int64(1)
	err := (&Runner{Client: client}).Apply(context.Background(), planner.ExecutionPlan{
		Actions: []planner.Action{{
			Op: planner.OpRollback,
			Object: planner.ObjectRef{
				Kind: "Deployment", Name: "api", Namespace: "default",
			},
			Revision: &rev,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := client.AppsV1().Deployments("default").Get(context.Background(), "api", metav1.GetOptions{})
	if got.Spec.Template.Spec.Containers[0].Image != "nginx:v1" {
		t.Fatalf("image=%s", got.Spec.Template.Spec.Containers[0].Image)
	}
}

func replicaSet(name, ns, ownerUID, revision, image, hash string) *appsv1.ReplicaSet {
	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name + "-" + hash,
			Namespace: ns,
			Annotations: map[string]string{
				revisionAnnotation: revision,
			},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       name,
				UID:        types.UID(ownerUID),
				Controller: boolPtr(true),
			}},
		},
		Spec: appsv1.ReplicaSetSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{
					"app": name,
					appsv1.DefaultDeploymentUniqueLabelKey: hash,
				}},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{Name: "app", Image: image}},
				},
			},
		},
	}
}

func boolPtr(v bool) *bool { return &v }
