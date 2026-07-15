package cluster

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/tools/clientcmd"
)

func TestFriendlier_nil(t *testing.T) {
	if Friendlier(nil) != nil {
		t.Fatal("expected nil")
	}
}

func TestFriendlier_unauthorized(t *testing.T) {
	err := Friendlier(apierrors.NewUnauthorized("bad token"))
	if !strings.Contains(err.Error(), "not authorized") {
		t.Fatalf("got %q", err.Error())
	}
	if !strings.Contains(err.Error(), usageGuide) {
		t.Fatalf("expected usage guide link, got %q", err.Error())
	}
	if !apierrors.IsUnauthorized(err) {
		t.Fatal("expected unwrap to preserve Unauthorized")
	}
}

func TestFriendlier_forbidden(t *testing.T) {
	raw := apierrors.NewForbidden(
		schema.GroupResource{Resource: "pods"},
		"",
		fmt.Errorf(`User "system:serviceaccount:default:kprompt" cannot list resource "pods" in API group "" in the namespace "default"`),
	)
	err := Friendlier(fmt.Errorf("query: %w", raw))
	msg := err.Error()
	if !strings.Contains(msg, "RBAC denied") {
		t.Fatalf("got %q", msg)
	}
	if !strings.Contains(msg, "list") || !strings.Contains(msg, "pods") || !strings.Contains(msg, "default") {
		t.Fatalf("expected verb/resource/ns hints, got %q", msg)
	}
	if !strings.Contains(msg, "kubectl auth can-i") {
		t.Fatalf("expected can-i hint, got %q", msg)
	}
	if !apierrors.IsForbidden(err) {
		t.Fatal("expected unwrap to preserve Forbidden")
	}
}

func TestFriendlier_missingContext(t *testing.T) {
	err := Friendlier(fmt.Errorf(`context "kind-missing" does not exist`))
	if !strings.Contains(err.Error(), `kube context "kind-missing" not found`) {
		t.Fatalf("got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "kubectl config get-contexts") {
		t.Fatalf("got %q", err.Error())
	}
}

func TestFriendlier_emptyConfig(t *testing.T) {
	err := Friendlier(fmt.Errorf("kube client config: %w", clientcmd.ErrEmptyConfig))
	if !strings.Contains(err.Error(), "no kubeconfig found") {
		t.Fatalf("got %q", err.Error())
	}
}

func TestFriendlier_missingFile(t *testing.T) {
	pathErr := &os.PathError{Op: "stat", Path: "/tmp/missing-kube", Err: os.ErrNotExist}
	err := Friendlier(fmt.Errorf("load kubeconfig: %w", pathErr))
	if !strings.Contains(err.Error(), "/tmp/missing-kube") {
		t.Fatalf("got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "KUBECONFIG") {
		t.Fatalf("got %q", err.Error())
	}
}

func TestFriendlier_connectionRefused(t *testing.T) {
	err := Friendlier(fmt.Errorf("Get \"https://127.0.0.1:6443\": dial tcp 127.0.0.1:6443: connect: connection refused"))
	if !strings.Contains(err.Error(), "connection refused") {
		t.Fatalf("got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "kubectl cluster-info") {
		t.Fatalf("got %q", err.Error())
	}
}

func TestFriendlier_timeout(t *testing.T) {
	err := Friendlier(&net.DNSError{Err: "i/o timeout", Name: "kubernetes.default", IsTimeout: true})
	if !strings.Contains(err.Error(), "timeout") {
		t.Fatalf("got %q", err.Error())
	}
}

func TestFriendlier_passthrough(t *testing.T) {
	orig := errors.New("something unrelated")
	err := Friendlier(orig)
	if err != orig {
		t.Fatalf("expected passthrough, got %v", err)
	}
}

func TestFriendlier_idempotent(t *testing.T) {
	once := Friendlier(apierrors.NewUnauthorized("x"))
	twice := Friendlier(once)
	if once != twice {
		t.Fatal("expected same friendly error instance")
	}
}

func TestParseForbiddenMessage(t *testing.T) {
	v, r, ns := parseForbiddenMessage(`User "x" cannot create resource "deployments" in API group "apps" in the namespace "prod"`)
	if v != "create" || r != "deployments" || ns != "prod" {
		t.Fatalf("got verb=%q resource=%q ns=%q", v, r, ns)
	}
}
