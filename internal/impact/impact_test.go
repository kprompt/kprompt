package impact

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestServiceImpactFindsConsumersBackendsAndIngress(t *testing.T) {
	client := fake.NewSimpleClientset(
		service("redis", map[string]string{"app": "redis"}),
		deployment("redis", map[string]string{"app": "redis"}, nil),
		deployment("orders", map[string]string{"app": "orders"}, []corev1.EnvVar{{
			Name: "REDIS_URL", Value: "redis://redis.payments.svc:6379",
		}}),
		ingress("redis-public", "redis"),
	)
	got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{
		Name: "redis", Namespace: "payments", Kind: "Service", Prompt: "who consumes redis",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Summary, "1 static consumer(s), 1 backend Deployment(s), 1 Ingress route(s)") {
		t.Fatalf("summary=%q", got.Summary)
	}
	requireFinding(t, got, "Impact.Consumer", "Deployment/orders consumes Service/redis")
	requireFinding(t, got, "Impact.Backend", "Deployment/redis is selected by Service/redis")
	requireFinding(t, got, "Impact.Ingress", "Ingress/redis-public routes to Service/redis")
	if got.Confidence != 0.8 {
		t.Fatalf("confidence=%v", got.Confidence)
	}
	if !contains(got.Degraded, "mesh") || !contains(got.Degraded, "otel") {
		t.Fatalf("mesh/otel still expected without Dynamic: %v", got.Degraded)
	}
}

func TestDeploymentImpactFindsServiceConsumersHPAAndPDB(t *testing.T) {
	min := int32(2)
	client := fake.NewSimpleClientset(
		deployment("api", map[string]string{"app": "api"}, nil),
		deployment("web", map[string]string{"app": "web"}, []corev1.EnvVar{{
			Name: "API_HOST", Value: "api",
		}}),
		service("api", map[string]string{"app": "api"}),
		&autoscalingv2.HorizontalPodAutoscaler{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
			Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
				ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{Kind: "Deployment", Name: "api"},
				MinReplicas:    &min, MaxReplicas: 10,
			},
			Status: autoscalingv2.HorizontalPodAutoscalerStatus{CurrentReplicas: 2, DesiredReplicas: 3},
		},
		&policyv1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{Name: "api", Namespace: "payments"},
			Spec: policyv1.PodDisruptionBudgetSpec{
				Selector:     &metav1.LabelSelector{MatchLabels: map[string]string{"app": "api"}},
				MinAvailable: &intstr.IntOrString{Type: intstr.Int, IntVal: 1},
			},
			Status: policyv1.PodDisruptionBudgetStatus{DisruptionsAllowed: 1, CurrentHealthy: 2, DesiredHealthy: 1},
		},
	)
	got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{
		Name: "api", Namespace: "payments", Kind: "Deployment", Prompt: "impact of deployment api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Summary, "1 consumer Deployment(s) via 1 Service(s), 1 HPA(s), 1 PDB(s)") {
		t.Fatalf("summary=%q", got.Summary)
	}
	requireFinding(t, got, "Impact.Consumer", "Deployment/web consumes Service/api")
	requireFinding(t, got, "Impact.Service", "Service/api selects Deployment/api")
	requireFinding(t, got, "Impact.HPA", "HorizontalPodAutoscaler/api scales Deployment/api")
	requireFinding(t, got, "Impact.PDB", "PodDisruptionBudget/api protects Deployment/api")
}

func TestServiceImpactReportsNoStaticConsumers(t *testing.T) {
	client := fake.NewSimpleClientset(service("isolated", nil))
	got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{
		Name: "isolated", Namespace: "payments", Kind: "Service",
	})
	if err != nil {
		t.Fatal(err)
	}
	requireFinding(t, got, "Impact.NoneFound", "No static consumers found")
	if got.Confidence != 0.55 {
		t.Fatalf("confidence=%v", got.Confidence)
	}
}

func TestReferencesServiceUsesTokens(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  bool
	}{
		{"redis://redis.payments.svc:6379", true},
		{"http://redis:6379", true},
		{"http://not-redis:6379", false},
		{"/api/v1/rediscover", false},
		{"http://example.test/redis/v1", false},
	} {
		if got := referencesService(tc.value, "redis", "payments"); got != tc.want {
			t.Errorf("referencesService(%q)=%v want %v", tc.value, got, tc.want)
		}
	}
}

func service(name string, selector map[string]string) runtime.Object {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "payments"},
		Spec:       corev1.ServiceSpec{Selector: selector},
	}
}

func deployment(name string, podLabels map[string]string, env []corev1.EnvVar) runtime.Object {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "payments"},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: podLabels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: podLabels},
				Spec: corev1.PodSpec{Containers: []corev1.Container{{
					Name: name, Image: "example/" + name, Env: env,
				}}},
			},
		},
	}
}

func ingress(name, serviceName string) runtime.Object {
	pathType := networkingv1.PathTypePrefix
	return &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: "payments"},
		Spec: networkingv1.IngressSpec{Rules: []networkingv1.IngressRule{{
			IngressRuleValue: networkingv1.IngressRuleValue{HTTP: &networkingv1.HTTPIngressRuleValue{
				Paths: []networkingv1.HTTPIngressPath{{
					Path: "/", PathType: &pathType,
					Backend: networkingv1.IngressBackend{Service: &networkingv1.IngressServiceBackend{
						Name: serviceName,
						Port: networkingv1.ServiceBackendPort{Number: 80},
					}},
				}},
			}},
		}}},
	}
}

func requireFinding(t *testing.T, got incident.Investigation, code, title string) {
	t.Helper()
	for _, f := range got.Findings {
		if f.Code == code && f.Title == title {
			return
		}
	}
	t.Fatalf("missing finding code=%q title=%q: %+v", code, title, got.Findings)
}
