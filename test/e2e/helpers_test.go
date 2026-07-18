//go:build e2e

package e2e_test

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"

	"github.com/kprompt/kprompt/internal/cluster"
	"github.com/kprompt/kprompt/internal/pipeline"
)

const (
	clusterName = "kprompt-e2e"
	ns          = "kprompt-e2e"
	deployName  = "demo"
	cmName      = "demo-config"
	secretName  = "demo-secret"
	widgetName  = "demo-widget"
	crdName     = "widgets.example.com"
	limitedSA   = "kprompt-e2e-limited"
)

func requireKind(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("kind"); err != nil {
		t.Skip("kind not installed")
	}
	if _, err := exec.LookPath("kubectl"); err != nil {
		t.Skip("kubectl not installed")
	}
}

func ensureKindCluster(t *testing.T, ctx context.Context) {
	t.Helper()
	out, err := exec.CommandContext(ctx, "kind", "get", "clusters").CombinedOutput()
	if err != nil {
		t.Fatalf("kind get clusters: %v (%s)", err, out)
	}
	if !bytes.Contains(out, []byte(clusterName)) {
		t.Logf("creating kind cluster %s …", clusterName)
		cmd := exec.CommandContext(ctx, "kind", "create", "cluster", "--name", clusterName)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("kind create cluster: %v", err)
		}
	}
}

func exportKubeconfig(t *testing.T, ctx context.Context) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "kubeconfig")
	cmd := exec.CommandContext(ctx, "kind", "export", "kubeconfig", "--name", clusterName, "--kubeconfig", path)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("export kubeconfig: %v (%s)", err, out)
	}
	return path
}

func restConfigFromKubeconfig(t *testing.T, path string) *rest.Config {
	t.Helper()
	cfg, err := clientcmd.BuildConfigFromFlags("", path)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func clientFromKubeconfig(t *testing.T, path string) *kubernetes.Clientset {
	t.Helper()
	cs, err := kubernetes.NewForConfig(restConfigFromKubeconfig(t, path))
	if err != nil {
		t.Fatal(err)
	}
	return cs
}

func pipelineKubeDeps(t *testing.T, path string) pipeline.Deps {
	t.Helper()
	cfg := restConfigFromKubeconfig(t, path)
	cs, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	dyn, err := dynamic.NewForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := cluster.NewResolverForConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline.Deps{Client: cs, Dynamic: dyn, Resolver: resolver}
}

func ensureNamespace(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	_, err := client.CoreV1().Namespaces().Get(ctx, ns, metav1.GetOptions{})
	if err == nil {
		return
	}
	_, err = client.CoreV1().Namespaces().Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: ns},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func ensureDeployment(t *testing.T, ctx context.Context, client kubernetes.Interface, replicas int32) {
	t.Helper()
	_, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err == nil {
		retryScale(t, ctx, client, replicas)
		return
	}
	labels := map[string]string{"app": deployName}
	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{Name: deployName, Namespace: ns},
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
	}
	if _, err := client.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func retryScale(t *testing.T, ctx context.Context, client kubernetes.Interface, replicas int32) {
	t.Helper()
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, deployName, metav1.GetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	dep.Spec.Replicas = &replicas
	if _, err := client.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func ensureConfigMap(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	_, err := client.CoreV1().ConfigMaps(ns).Get(ctx, cmName, metav1.GetOptions{})
	if err == nil {
		return
	}
	_, err = client.CoreV1().ConfigMaps(ns).Create(ctx, &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: cmName, Namespace: ns},
		Data:       map[string]string{"key": "value"},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func ensureSecret(t *testing.T, ctx context.Context, client kubernetes.Interface) {
	t.Helper()
	_, err := client.CoreV1().Secrets(ns).Get(ctx, secretName, metav1.GetOptions{})
	if err == nil {
		return
	}
	_, err = client.CoreV1().Secrets(ns).Create(ctx, &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: secretName, Namespace: ns},
		Type:       corev1.SecretTypeOpaque,
		Data:       map[string][]byte{"password": []byte("s3cret")},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func ensureWidgetCRD(t *testing.T, ctx context.Context, restCfg *rest.Config) {
	t.Helper()
	dyn, err := dynamic.NewForConfig(restCfg)
	if err != nil {
		t.Fatal(err)
	}
	crdGVR := schema.GroupVersionResource{
		Group: "apiextensions.k8s.io", Version: "v1", Resource: "customresourcedefinitions",
	}
	_, err = dyn.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		crd := &unstructured.Unstructured{Object: map[string]any{
			"apiVersion": "apiextensions.k8s.io/v1",
			"kind":       "CustomResourceDefinition",
			"metadata":   map[string]any{"name": crdName},
			"spec": map[string]any{
				"group": "example.com",
				"names": map[string]any{
					"plural":   "widgets",
					"singular": "widget",
					"kind":     "Widget",
					"listKind": "WidgetList",
				},
				"scope": "Namespaced",
				"versions": []any{
					map[string]any{
						"name":    "v1",
						"served":  true,
						"storage": true,
						"schema": map[string]any{
							"openAPIV3Schema": map[string]any{
								"type": "object",
								"properties": map[string]any{
									"spec": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"color": map[string]any{"type": "string"},
										},
									},
									"status": map[string]any{
										"type": "object",
										"properties": map[string]any{
											"phase": map[string]any{"type": "string"},
										},
									},
								},
							},
						},
					},
				},
			},
		}}
		if _, err := dyn.Resource(crdGVR).Create(ctx, crd, metav1.CreateOptions{}); err != nil {
			t.Fatal(err)
		}
	} else if err != nil {
		t.Fatal(err)
	}

	err = wait.PollUntilContextTimeout(ctx, time.Second, 60*time.Second, true, func(ctx context.Context) (bool, error) {
		got, err := dyn.Resource(crdGVR).Get(ctx, crdName, metav1.GetOptions{})
		if err != nil {
			return false, nil
		}
		conds, _, _ := unstructured.NestedSlice(got.Object, "status", "conditions")
		for _, c := range conds {
			m, ok := c.(map[string]any)
			if !ok {
				continue
			}
			if m["type"] == "Established" && m["status"] == "True" {
				return true, nil
			}
		}
		return false, nil
	})
	if err != nil {
		t.Fatalf("wait for CRD established: %v", err)
	}

	widgetGVR := schema.GroupVersionResource{Group: "example.com", Version: "v1", Resource: "widgets"}
	_, err = dyn.Resource(widgetGVR).Namespace(ns).Get(ctx, widgetName, metav1.GetOptions{})
	if err == nil {
		return
	}
	obj := &unstructured.Unstructured{Object: map[string]any{
		"apiVersion": "example.com/v1",
		"kind":       "Widget",
		"metadata": map[string]any{
			"name":      widgetName,
			"namespace": ns,
		},
		"spec": map[string]any{
			"color": "blue",
		},
		"status": map[string]any{
			"phase": "Ready",
		},
	}}
	if _, err := dyn.Resource(widgetGVR).Namespace(ns).Create(ctx, obj, metav1.CreateOptions{}); err != nil {
		t.Fatal(err)
	}
}

func ensureLimitedSecretDeniedKubeconfig(t *testing.T, ctx context.Context, admin *kubernetes.Clientset, adminKubeconfig string) string {
	t.Helper()
	ensureNamespace(t, ctx, admin)

	_, err := admin.CoreV1().ServiceAccounts(ns).Get(ctx, limitedSA, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = admin.CoreV1().ServiceAccounts(ns).Create(ctx, &corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{Name: limitedSA, Namespace: ns},
		}, metav1.CreateOptions{})
	}
	if err != nil {
		t.Fatal(err)
	}

	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: limitedSA, Namespace: ns},
		Rules: []rbacv1.PolicyRule{{
			APIGroups: []string{""},
			Resources: []string{"pods", "configmaps"},
			Verbs:     []string{"get", "list"},
		}},
	}
	_, err = admin.RbacV1().Roles(ns).Get(ctx, limitedSA, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = admin.RbacV1().Roles(ns).Create(ctx, role, metav1.CreateOptions{})
	} else if err == nil {
		_, err = admin.RbacV1().Roles(ns).Update(ctx, role, metav1.UpdateOptions{})
	}
	if err != nil {
		t.Fatal(err)
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: limitedSA, Namespace: ns},
		RoleRef:    rbacv1.RoleRef{APIGroup: "rbac.authorization.k8s.io", Kind: "Role", Name: limitedSA},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: limitedSA, Namespace: ns}},
	}
	_, err = admin.RbacV1().RoleBindings(ns).Get(ctx, limitedSA, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = admin.RbacV1().RoleBindings(ns).Create(ctx, rb, metav1.CreateOptions{})
	}
	if err != nil {
		t.Fatal(err)
	}

	exp := int64(3600)
	tok, err := admin.CoreV1().ServiceAccounts(ns).CreateToken(ctx, limitedSA, &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{ExpirationSeconds: &exp},
	}, metav1.CreateOptions{})
	if err != nil {
		t.Fatal(err)
	}

	adminCfg, err := clientcmd.LoadFromFile(adminKubeconfig)
	if err != nil {
		t.Fatal(err)
	}
	ctxName := adminCfg.CurrentContext
	clName := adminCfg.Contexts[ctxName].Cluster
	cluster := adminCfg.Clusters[clName]

	out := clientcmdapi.NewConfig()
	out.Clusters[clName] = cluster
	out.AuthInfos[limitedSA] = &clientcmdapi.AuthInfo{Token: tok.Status.Token}
	out.Contexts[limitedSA] = &clientcmdapi.Context{
		Cluster:  clName,
		AuthInfo: limitedSA,
	}
	out.CurrentContext = limitedSA
	path := filepath.Join(t.TempDir(), "limited.kubeconfig")
	if err := clientcmd.WriteToFile(*out, path); err != nil {
		t.Fatal(err)
	}
	return path
}
