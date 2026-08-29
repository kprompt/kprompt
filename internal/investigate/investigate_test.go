package investigate

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/incident"
	toolprometheus "github.com/kprompt/kprompt/internal/tools/prometheus"
)

func TestRunCrashLoopWithServiceNoEndpoints(t *testing.T) {
	ns := "payments"
	labels := map[string]string{"app": "api"}
	var replicas int32 = 1

	objs := []runtime.Object{
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns, UID: "dep1"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "api", Image: "busybox:1.36"}},
					},
				},
			},
			Status: appsv1.DeploymentStatus{ReadyReplicas: 0, UnavailableReplicas: 1},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-rs",
				Namespace: ns,
				UID:       "rs1",
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "Deployment", Name: "api", UID: "dep1",
				}},
			},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "api-abc",
				Namespace: ns,
				Labels:    labels,
			},
			Status: corev1.PodStatus{
				Phase: corev1.PodRunning,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name:         "api",
					Ready:        false,
					RestartCount: 5,
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "CrashLoopBackOff"},
					},
				}},
			},
		},
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
			Spec: corev1.ServiceSpec{
				Selector: labels,
				Ports:    []corev1.ServicePort{{Port: 80, Protocol: corev1.ProtocolTCP}},
			},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
			Subsets:    []corev1.EndpointSubset{},
		},
	}

	client := fake.NewSimpleClientset(objs...)
	doc, rep, err := (&Investigator{Client: client}).Run(context.Background(), Request{
		Name: "api", Namespace: ns, Kind: "Deployment",
		Prompt: "investigate api in payments",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := incident.ValidateInvestigation(doc); err != nil {
		t.Fatal(err)
	}
	if doc.Target == nil || doc.Target.Name != "api" {
		t.Fatalf("target: %+v", doc.Target)
	}
	if !hasFinding(doc, "NoReadyEndpoints") {
		t.Fatalf("expected NoReadyEndpoints: %+v", doc.Findings)
	}
	if !hasFinding(doc, "CrashLoopBackOff") {
		t.Fatalf("expected CrashLoopBackOff: %+v", doc.Findings)
	}
	if !chainHas(rep, "Service", "api") || !chainHas(rep, "Endpoints", "api") {
		t.Fatalf("chain missing service hops: %+v", rep.Chain)
	}
	if doc.Confidence <= 0 {
		t.Fatalf("confidence: %v", doc.Confidence)
	}
	// Ingress listed successfully (none matched) → omit; mesh + prometheus still gaps.
	if contains(doc.Degraded, "ingress") {
		t.Fatalf("ingress should not be degraded after successful walk: %v", doc.Degraded)
	}
	for _, d := range []string{"mesh", "prometheus"} {
		if !contains(doc.Degraded, d) {
			t.Fatalf("degraded missing %s: %v", d, doc.Degraded)
		}
	}
}

func TestRunImagePullRootCause(t *testing.T) {
	ns := "payments"
	labels := map[string]string{"app": "worker"}
	var replicas int32 = 1
	client := fake.NewSimpleClientset(
		&appsv1.Deployment{
			ObjectMeta: metav1.ObjectMeta{Name: "worker", Namespace: ns, UID: "d1"},
			Spec: appsv1.DeploymentSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
				Template: corev1.PodTemplateSpec{
					ObjectMeta: metav1.ObjectMeta{Labels: labels},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{{Name: "worker", Image: "missing:9.9.9"}},
					},
				},
			},
		},
		&appsv1.ReplicaSet{
			ObjectMeta: metav1.ObjectMeta{
				Name: "worker-rs", Namespace: ns,
				OwnerReferences: []metav1.OwnerReference{{
					APIVersion: "apps/v1", Kind: "Deployment", Name: "worker", UID: "d1",
				}},
			},
			Spec: appsv1.ReplicaSetSpec{
				Replicas: &replicas,
				Selector: &metav1.LabelSelector{MatchLabels: labels},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{Name: "worker-1", Namespace: ns, Labels: labels},
			Status: corev1.PodStatus{
				Phase: corev1.PodPending,
				ContainerStatuses: []corev1.ContainerStatus{{
					Name: "worker",
					State: corev1.ContainerState{
						Waiting: &corev1.ContainerStateWaiting{Reason: "ImagePullBackOff"},
					},
				}},
			},
		},
	)
	doc, _, err := (&Investigator{Client: client}).Run(context.Background(), Request{
		Name: "worker", Namespace: ns, Kind: "Deployment",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !hasFinding(doc, "ImagePullBackOff") {
		t.Fatalf("findings: %+v root=%s", doc.Findings, doc.RootCause)
	}
	if doc.RootCause == "" {
		t.Fatal("empty root cause")
	}
}

func TestRunParallelServiceEndpoints(t *testing.T) {
	ns := "payments"
	labels := map[string]string{"app": "api"}
	var replicas int32 = 1
	client := fake.NewSimpleClientset(
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
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{Name: "api-headless", Namespace: ns},
			Spec:       corev1.ServiceSpec{Selector: labels, Ports: []corev1.ServicePort{{Port: 8080}}},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: ns},
			Subsets: []corev1.EndpointSubset{{
				Addresses: []corev1.EndpointAddress{{IP: "10.0.0.1"}},
			}},
		},
		&corev1.Endpoints{
			ObjectMeta: metav1.ObjectMeta{Name: "api-headless", Namespace: ns},
			Subsets:    []corev1.EndpointSubset{},
		},
	)
	doc, rep, err := (&Investigator{Client: client}).Run(context.Background(), Request{
		Name: "api", Namespace: ns, Kind: "Deployment", Prompt: "investigate api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !chainHas(rep, "Service", "api") || !chainHas(rep, "Service", "api-headless") {
		t.Fatalf("expected both services in chain: %+v", rep.Chain)
	}
	if !chainHas(rep, "Endpoints", "api") || !chainHas(rep, "Endpoints", "api-headless") {
		t.Fatalf("expected both endpoints hops: %+v", rep.Chain)
	}
	if !hasFinding(doc, "NoReadyEndpoints") {
		t.Fatalf("expected NoReadyEndpoints for headless empty: %+v", doc.Findings)
	}
	if err := incident.ValidateInvestigation(doc); err != nil {
		t.Fatal(err)
	}
}

func TestRunIngressAttached(t *testing.T) {
	ns := "payments"
	labels := map[string]string{"app": "api"}
	var replicas int32 = 1
	class := "nginx"
	client := fake.NewSimpleClientset(
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
		&networkingv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Name: "api-ing", Namespace: ns},
			Spec: networkingv1.IngressSpec{
				IngressClassName: &class,
				Rules: []networkingv1.IngressRule{{
					Host: "api.example.com",
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{{
								Path:     "/",
								PathType: ptrPathType(networkingv1.PathTypePrefix),
								Backend: networkingv1.IngressBackend{
									Service: &networkingv1.IngressServiceBackend{
										Name: "api",
										Port: networkingv1.ServiceBackendPort{Number: 80},
									},
								},
							}},
						},
					},
				}},
			},
		},
	)
	doc, rep, err := (&Investigator{Client: client}).Run(context.Background(), Request{
		Name: "api", Namespace: ns, Kind: "Deployment", Prompt: "investigate api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !chainHas(rep, "Ingress", "api-ing") {
		t.Fatalf("chain missing ingress: %+v", rep.Chain)
	}
	if !hasFinding(doc, "IngressAttached") {
		t.Fatalf("findings: %+v", doc.Findings)
	}
	if contains(doc.Degraded, "ingress") {
		t.Fatalf("ingress should not be degraded: %v", doc.Degraded)
	}
	if !contains(doc.Degraded, "mesh") || !contains(doc.Degraded, "prometheus") {
		t.Fatalf("mesh/prometheus still expected: %v", doc.Degraded)
	}
}

func TestRunPrometheusMetrics(t *testing.T) {
	ns := "payments"
	labels := map[string]string{"app": "api"}
	var replicas int32 = 1
	client := fake.NewSimpleClientset(
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
			Status:     corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)
	q := stubMetrics{vals: map[string]float64{
		"cpu_usage":           0.12,
		"memory_working_set":  64e6,
		"restart_rate":        0.01,
	}}
	doc, _, err := (&Investigator{Client: client, Metrics: q}).Run(context.Background(), Request{
		Name: "api", Namespace: ns, Kind: "Deployment",
	})
	if err != nil {
		t.Fatal(err)
	}
	var metrics int
	for _, e := range doc.Evidence {
		if e.Type == incident.EvidenceMetric {
			metrics++
		}
	}
	if metrics < 1 {
		t.Fatalf("expected metric evidence: %+v", doc.Evidence)
	}
	if contains(doc.Degraded, "prometheus") {
		t.Fatalf("prometheus should not be degraded: %v", doc.Degraded)
	}
}

type stubMetrics struct {
	vals map[string]float64
}

func (s stubMetrics) Query(_ context.Context, promQL string, _ time.Time) (toolprometheus.Result, error) {
	for reason, v := range s.vals {
		if strings.Contains(promQL, reason) ||
			(reason == "cpu_usage" && strings.Contains(promQL, "container_cpu_usage")) ||
			(reason == "memory_working_set" && strings.Contains(promQL, "container_memory_working_set")) ||
			(reason == "restart_rate" && strings.Contains(promQL, "kube_pod_container_status_restarts")) {
			return toolprometheus.Result{
				Type:   "vector",
				Series: []toolprometheus.Series{{Samples: []toolprometheus.Sample{{Value: fmt.Sprintf("%g", v)}}}},
			}, nil
		}
	}
	return toolprometheus.Result{}, fmt.Errorf("no series")
}

func ptrPathType(t networkingv1.PathType) *networkingv1.PathType { return &t }

func hasFinding(doc incident.Investigation, code string) bool {
	for _, f := range doc.Findings {
		if f.Code == code {
			return true
		}
	}
	return false
}

func chainHas(rep cluster.ExplainReport, level, name string) bool {
	for _, s := range rep.Chain {
		if s.Level == level && s.Name == name {
			return true
		}
	}
	return false
}

func contains(ss []string, want string) bool {
	for _, s := range ss {
		if s == want {
			return true
		}
	}
	return false
}
