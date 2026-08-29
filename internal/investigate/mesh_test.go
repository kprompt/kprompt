package investigate

import (
	"context"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	metafake "k8s.io/client-go/testing"

	toolistio "github.com/kprompt/kprompt/internal/tools/istio"
)

func TestRunVirtualServiceAttached(t *testing.T) {
	ns := "payments"
	labels := map[string]string{"app": "api"}
	var replicas int32 = 1
	client := kubefake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns, UID: "dep1"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "busybox"}}},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 1},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "api-0", Namespace: ns, Labels: labels},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "api", Ready: true,
					State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{}},
				}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
			Spec:       corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Port: 80}}},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
			}},
		},
	)

	gvr := toolistio.VirtualServiceGVR
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "VirtualServiceList"},
	)
	dyn.PrependReactor("list", "virtualservices", func(action metafake.Action) (bool, runtime.Object, error) {
		list := &unstructured.UnstructuredList{Items: []unstructured.Unstructured{{
			Object: map[string]any{
				"apiVersion": "networking.istio.io/v1beta1",
				"kind":       "VirtualService",
				"metadata":   map[string]any{"name": "api-vs", "namespace": ns},
				"spec": map[string]any{
					"hosts": []any{"api.payments.svc.cluster.local"},
					"http": []any{
						map[string]any{
							"route": []any{
								map[string]any{
									"destination": map[string]any{"host": "api"},
									"weight":      int64(100),
								},
							},
						},
					},
				},
			},
		}}}
		list.SetGroupVersionKind(schema.GroupVersionKind{
			Group: gvr.Group, Version: gvr.Version, Kind: "VirtualServiceList",
		})
		return true, list, nil
	})

	doc, rep, err := (&Investigator{Client: client, Dynamic: dyn}).Run(context.Background(), Request{
		Name: "api", Namespace: ns, Kind: "Deployment", Prompt: "investigate api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !chainHas(rep, "VirtualService", "api-vs") {
		t.Fatalf("chain missing VirtualService: %+v", rep.Chain)
	}
	if !hasFinding(doc, "VirtualServiceAttached") {
		t.Fatalf("findings: %+v", doc.Findings)
	}
	if contains(doc.Degraded, "mesh") {
		t.Fatalf("mesh should not be degraded after walk: %v", doc.Degraded)
	}
	if !contains(doc.Degraded, "prometheus") {
		t.Fatalf("prometheus still expected: %v", doc.Degraded)
	}
}

func TestRunMeshWalkedEmptyWhenNoCRD(t *testing.T) {
	ns := "payments"
	labels := map[string]string{"app": "api"}
	var replicas int32 = 1
	client := kubefake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns, UID: "dep1"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec:       corev1.PodSpec{Containers: []corev1.Container{{Name: "api", Image: "busybox"}}},
				},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
			Spec:       corev1.ServiceSpec{Selector: labels},
		},
	)
	gvr := toolistio.VirtualServiceGVR
	dyn := fake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(),
		map[schema.GroupVersionResource]string{gvr: "VirtualServiceList"},
	)
	dyn.PrependReactor("list", "virtualservices", func(action metafake.Action) (bool, runtime.Object, error) {
		return true, nil, errNoMatchGVR{}
	})

	doc, _, err := (&Investigator{Client: client, Dynamic: dyn}).Run(context.Background(), Request{
		Name: "api", Namespace: ns, Kind: "Deployment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if contains(doc.Degraded, "mesh") {
		t.Fatalf("absent CRD should clear mesh degraded: %v", doc.Degraded)
	}
}

type errNoMatchGVR struct{}

func (errNoMatchGVR) Error() string {
	return "no matches for kind VirtualService in version networking.istio.io/v1beta1"
}

func TestMatchMeshHostToService(t *testing.T) {
	svcs := []string{"api"}
	cases := []struct {
		host string
		want string
	}{
		{"api", "api"},
		{"api.payments", "api"},
		{"api.payments.svc", "api"},
		{"api.payments.svc.cluster.local", "api"},
		{"api.payments.svc.cluster.local:8080", "api"},
		{"other", ""},
		{"api.other", ""},
	}
	for _, tc := range cases {
		if got := toolistio.MatchHostToService(tc.host, "payments", svcs); got != tc.want {
			t.Fatalf("%q: got %q want %q", tc.host, got, tc.want)
		}
	}
}
