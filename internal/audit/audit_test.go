package audit

import (
	"context"
	"strings"
	"testing"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/kprompt/kprompt/internal/incident"
)

func TestAuditFindsHygieneIssues(t *testing.T) {
	priv := true
	esc := true
	client := fake.NewSimpleClientset(
		deployment("bad", "payments", corev1.PodSpec{
			HostNetwork: true,
			Containers: []corev1.Container{{
				Name:            "app",
				Image:           "nginx:latest",
				ImagePullPolicy: "",
				SecurityContext: &corev1.SecurityContext{
					Privileged:               &priv,
					AllowPrivilegeEscalation: &esc,
				},
			}},
		}),
		deployment("good", "payments", corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true)},
			Containers: []corev1.Container{{
				Name:            "app",
				Image:           "nginx:1.27-alpine",
				ImagePullPolicy: corev1.PullIfNotPresent,
				SecurityContext: &corev1.SecurityContext{
					RunAsNonRoot:             boolPtr(true),
					AllowPrivilegeEscalation: boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(true),
				},
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
		}),
	)

	got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{
		Namespace: "payments",
		Prompt:    "audit payments namespace",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got.Summary, "hygiene finding(s) across 2 workload(s)") {
		t.Fatalf("summary=%q", got.Summary)
	}
	requireFinding(t, got, "Audit.Privileged", "Deployment/bad container app is privileged")
	requireFinding(t, got, "Audit.HostNamespace", "Deployment/bad shares host namespaces")
	requireFinding(t, got, "Audit.LatestTag", "Deployment/bad container app uses a mutable image tag")
	requireFinding(t, got, "Audit.MissingLimits", "Deployment/bad container app is missing CPU/memory limits")
	requireFinding(t, got, "Audit.RunAsRoot", "Deployment/bad container app may run as root")
	requireFinding(t, got, "Audit.WritableRootFS", "Deployment/bad container app has a writable root filesystem")
	if findingTitle(got, "Audit.Privileged", "Deployment/good") {
		t.Fatal("clean deployment should not be privileged")
	}
	if findingTitle(got, "Audit.WritableRootFS", "Deployment/good") {
		t.Fatal("clean deployment sets readOnlyRootFilesystem=true and should not be flagged")
	}
	if got.Confidence != 0.9 {
		t.Fatalf("confidence=%v", got.Confidence)
	}
}

func TestAuditCleanNamespace(t *testing.T) {
	client := fake.NewSimpleClientset(
		deployment("api", "prod", corev1.PodSpec{
			SecurityContext: &corev1.PodSecurityContext{RunAsNonRoot: boolPtr(true)},
			Containers: []corev1.Container{{
				Name:            "api",
				Image:           "ghcr.io/acme/api@sha256:deadbeef",
				ImagePullPolicy: corev1.PullIfNotPresent,
				SecurityContext: &corev1.SecurityContext{
					RunAsNonRoot:             boolPtr(true),
					AllowPrivilegeEscalation: boolPtr(false),
					ReadOnlyRootFilesystem:   boolPtr(true),
				},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("50m"),
						corev1.ResourceMemory: resource.MustParse("64Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
				},
			}},
		}),
	)
	got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{
		Namespace: "prod", Prompt: "security scan",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Findings) != 0 {
		t.Fatalf("findings=%+v", got.Findings)
	}
	if !strings.Contains(got.Summary, "no issues matched MVP rules") {
		t.Fatalf("summary=%q", got.Summary)
	}
}

func TestAuditWritableRootFS(t *testing.T) {
	cases := []struct {
		name     string
		roFS     *bool // nil = unset
		wantFlag bool
	}{
		{"readonly-true", boolPtr(true), false},
		{"readonly-false", boolPtr(false), true},
		{"unset", nil, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			client := fake.NewSimpleClientset(
				deployment(tc.name, "prod", corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "app",
						Image: "nginx:1.27",
						SecurityContext: &corev1.SecurityContext{
							ReadOnlyRootFilesystem: tc.roFS,
						},
					}},
				}),
			)
			got, err := (&Analyzer{Client: client}).Run(context.Background(), Request{Namespace: "prod"})
			if err != nil {
				t.Fatal(err)
			}
			flagged := findingTitle(got, "Audit.WritableRootFS", "Deployment/"+tc.name)
			if flagged != tc.wantFlag {
				t.Fatalf("writable-root-fs flagged=%v want=%v; findings=%+v", flagged, tc.wantFlag, got.Findings)
			}
		})
	}
}

func TestImageIsLatestOrUntagged(t *testing.T) {
	cases := map[string]bool{
		"nginx":                       true,
		"nginx:latest":                true,
		"nginx:1.27":                  false,
		"ghcr.io/acme/api@sha256:abc": false,
	}
	for image, want := range cases {
		if got := imageIsLatestOrUntagged(image); got != want {
			t.Fatalf("%s: got %v want %v", image, got, want)
		}
	}
}

func deployment(name, ns string, spec corev1.PodSpec) *appsv1.Deployment {
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": name}},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{"app": name}},
				Spec:       spec,
			},
		},
	}
}

func boolPtr(v bool) *bool { return &v }

func requireFinding(t *testing.T, got incident.Investigation, code, titleSubstr string) {
	t.Helper()
	for _, f := range got.Findings {
		if f.Code == code && strings.Contains(f.Title, titleSubstr) {
			return
		}
	}
	t.Fatalf("missing finding code=%s title~%q in %+v", code, titleSubstr, got.Findings)
}

func findingTitle(got incident.Investigation, code, titleSubstr string) bool {
	for _, f := range got.Findings {
		if f.Code == code && strings.Contains(f.Title, titleSubstr) {
			return true
		}
	}
	return false
}
