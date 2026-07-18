package optimize

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestCollectInventoryDeploymentsAndStatefulSets(t *testing.T) {
	replicas := int32(3)
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "prod"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "api",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						}},
					},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 2},
		},
		&appsv1.StatefulSet{
			ObjectMeta: metav1.ObjectMeta{Name: "db", Namespace: "prod"},
			Spec: appsv1.StatefulSetSpec{
				Replicas: &replicas,
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{
							Name: "db",
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						}},
					},
				},
			},
			Status: appsv1.StatefulSetStatus{ReadyReplicas: 3},
		},
	)

	inv, err := CollectInventory(context.Background(), client, Request{Namespace: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Workloads) != 2 || inv.Namespaces != 1 {
		t.Fatalf("inv=%+v", inv)
	}
	var api, db *Workload
	for i := range inv.Workloads {
		switch inv.Workloads[i].Name {
		case "api":
			api = &inv.Workloads[i]
		case "db":
			db = &inv.Workloads[i]
		}
	}
	if api == nil || api.Kind != WorkloadDeployment || api.Replicas != 3 || api.ReadyReplicas != 2 {
		t.Fatalf("api=%+v", api)
	}
	if api.CPURequest == "" || api.MemoryLimit == "" || api.MissingReq || api.MissingLim {
		t.Fatalf("api resources=%+v", api)
	}
	if db == nil || db.Kind != WorkloadStatefulSet || !db.MissingReq || !db.MissingLim {
		t.Fatalf("db=%+v", db)
	}

	rep := BuildScaffold(Request{Namespace: "prod"})
	ApplyInventory(&rep, inv)
	if rep.Sections.Inventory.Status != SectionReady {
		t.Fatalf("section=%+v", rep.Sections.Inventory)
	}
	if len(rep.Workloads) != 2 {
		t.Fatalf("workloads=%d", len(rep.Workloads))
	}
	foundMissing := false
	for _, f := range rep.Findings {
		if f.Code == "optimize.inventory.missing_requests" {
			foundMissing = true
		}
	}
	if !foundMissing {
		t.Fatalf("findings=%+v", rep.Findings)
	}
	if !strings.Contains(rep.Summary, "2 workloads") {
		t.Fatalf("summary=%s", rep.Summary)
	}
}

func TestCollectInventoryClusterScope(t *testing.T) {
	replicas := int32(1)
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "a", Namespace: "ns-a"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "b", Namespace: "ns-b"},
			Spec:       appsv1.DeploymentSpec{Replicas: &replicas},
		},
	)
	inv, err := CollectInventory(context.Background(), client, Request{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Workloads) != 2 || inv.Namespaces != 2 {
		t.Fatalf("inv=%+v", inv)
	}
}

func TestCollectInventoryRequiresClient(t *testing.T) {
	_, err := CollectInventory(context.Background(), nil, Request{})
	if err == nil {
		t.Fatal("expected error")
	}
}
