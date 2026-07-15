package planner

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"sigs.k8s.io/yaml"
)

// EnrichDiffs replaces Action.Diff with live before→after previews when the
// cluster is reachable. Failures are ignored so plan printing still works.
func EnrichDiffs(ctx context.Context, client kubernetes.Interface, plan *ExecutionPlan) {
	if client == nil || plan == nil {
		return
	}
	for i := range plan.Actions {
		a := &plan.Actions[i]
		switch a.Op {
		case OpScale:
			enrichScaleDiff(ctx, client, a)
		case OpCreate, OpUpdate:
			enrichManifestDiff(ctx, client, a)
		case OpRollback:
			enrichRollbackDiff(ctx, client, a)
		case OpDelete:
			enrichDeleteDiff(ctx, client, a)
		}
	}
}

func enrichScaleDiff(ctx context.Context, client kubernetes.Interface, a *Action) {
	if a.Replicas == nil {
		return
	}
	ns := namespaceOrDefault(a.Object.Namespace)
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		a.Diff = fmt.Sprintf("replicas: (missing) → %d\n(note: Deployment/%s not found)", *a.Replicas, a.Object.Name)
		return
	}
	if err != nil {
		return
	}
	cur := int32(1)
	if dep.Spec.Replicas != nil {
		cur = *dep.Spec.Replicas
	}
	a.Diff = fmt.Sprintf("replicas: %d → %d", cur, *a.Replicas)
}

func enrichManifestDiff(ctx context.Context, client kubernetes.Interface, a *Action) {
	ns := namespaceOrDefault(a.Object.Namespace)
	switch a.Object.Kind {
	case "Deployment":
		enrichDeploymentManifestDiff(ctx, client, ns, a)
	case "Service":
		enrichServiceManifestDiff(ctx, client, ns, a)
	}
}

func enrichDeploymentManifestDiff(ctx context.Context, client kubernetes.Interface, ns string, a *Action) {
	desired, err := decodeDeployment(a.Manifest)
	if err != nil {
		return
	}
	existing, err := client.AppsV1().Deployments(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		a.Op = OpCreate
		a.Diff = formatCreateDeployment(desired)
		return
	}
	if err != nil {
		return
	}
	a.Op = OpUpdate
	a.Diff = formatUpdateDeployment(existing, desired)
}

func enrichServiceManifestDiff(ctx context.Context, client kubernetes.Interface, ns string, a *Action) {
	desired, err := decodeService(a.Manifest)
	if err != nil {
		return
	}
	existing, err := client.CoreV1().Services(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		a.Op = OpCreate
		a.Diff = formatCreateService(desired)
		return
	}
	if err != nil {
		return
	}
	a.Op = OpUpdate
	a.Diff = formatUpdateService(existing, desired)
}

func enrichRollbackDiff(ctx context.Context, client kubernetes.Interface, a *Action) {
	ns := namespaceOrDefault(a.Object.Namespace)
	dep, err := client.AppsV1().Deployments(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
	if err != nil {
		return
	}
	curImg := firstContainerImage(dep.Spec.Template.Spec.Containers)
	curRev := dep.Annotations["deployment.kubernetes.io/revision"]
	if curRev == "" {
		curRev = "?"
	}
	target := "previous"
	if a.Revision != nil {
		target = fmt.Sprintf("%d", *a.Revision)
	}
	lines := []string{
		fmt.Sprintf("revision: %s → %s", curRev, target),
	}
	if curImg != "" {
		lines = append(lines, fmt.Sprintf("image now: %s", curImg))
	}
	a.Diff = strings.Join(lines, "\n")
}

func enrichDeleteDiff(ctx context.Context, client kubernetes.Interface, a *Action) {
	ns := namespaceOrDefault(a.Object.Namespace)
	exists := false
	var detail string
	switch a.Object.Kind {
	case "Deployment":
		dep, err := client.AppsV1().Deployments(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
		if err == nil {
			exists = true
			detail = fmt.Sprintf("image: %s", firstContainerImage(dep.Spec.Template.Spec.Containers))
		}
	case "Service":
		svc, err := client.CoreV1().Services(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
		if err == nil {
			exists = true
			detail = fmt.Sprintf("type: %s clusterIP: %s", svc.Spec.Type, svc.Spec.ClusterIP)
		}
	case "Pod":
		pod, err := client.CoreV1().Pods(ns).Get(ctx, a.Object.Name, metav1.GetOptions{})
		if err == nil {
			exists = true
			detail = fmt.Sprintf("phase: %s", pod.Status.Phase)
		}
	}
	if !exists {
		a.Diff = fmt.Sprintf("- %s/%s (not found)", a.Object.Kind, a.Object.Name)
		return
	}
	a.Diff = fmt.Sprintf("- %s/%s\n  %s", a.Object.Kind, a.Object.Name, detail)
}

func formatCreateDeployment(dep *appsv1.Deployment) string {
	replicas := int32(1)
	if dep.Spec.Replicas != nil {
		replicas = *dep.Spec.Replicas
	}
	img := firstContainerImage(dep.Spec.Template.Spec.Containers)
	return fmt.Sprintf("+ Deployment/%s (create)\n  image: %s\n  replicas: %d", dep.Name, img, replicas)
}

func formatUpdateDeployment(cur, desired *appsv1.Deployment) string {
	curRep := int32(1)
	if cur.Spec.Replicas != nil {
		curRep = *cur.Spec.Replicas
	}
	newRep := curRep
	if desired.Spec.Replicas != nil {
		newRep = *desired.Spec.Replicas
	}
	curImg := firstContainerImage(cur.Spec.Template.Spec.Containers)
	newImg := firstContainerImage(desired.Spec.Template.Spec.Containers)
	lines := []string{fmt.Sprintf("~ Deployment/%s (update)", desired.Name)}
	if curImg != newImg {
		lines = append(lines, fmt.Sprintf("  image: %s → %s", curImg, newImg))
	} else {
		lines = append(lines, fmt.Sprintf("  image: %s (unchanged)", curImg))
	}
	if curRep != newRep {
		lines = append(lines, fmt.Sprintf("  replicas: %d → %d", curRep, newRep))
	} else {
		lines = append(lines, fmt.Sprintf("  replicas: %d (unchanged)", curRep))
	}
	return strings.Join(lines, "\n")
}

func formatCreateService(svc *corev1.Service) string {
	port := "-"
	if len(svc.Spec.Ports) > 0 {
		port = fmt.Sprintf("%d", svc.Spec.Ports[0].Port)
	}
	return fmt.Sprintf("+ Service/%s (create)\n  port: %s\n  type: %s", svc.Name, port, svc.Spec.Type)
}

func formatUpdateService(cur, desired *corev1.Service) string {
	curPort, newPort := "-", "-"
	if len(cur.Spec.Ports) > 0 {
		curPort = fmt.Sprintf("%d", cur.Spec.Ports[0].Port)
	}
	if len(desired.Spec.Ports) > 0 {
		newPort = fmt.Sprintf("%d", desired.Spec.Ports[0].Port)
	}
	lines := []string{fmt.Sprintf("~ Service/%s (update)", desired.Name)}
	if curPort != newPort {
		lines = append(lines, fmt.Sprintf("  port: %s → %s", curPort, newPort))
	} else {
		lines = append(lines, fmt.Sprintf("  port: %s (unchanged)", curPort))
	}
	return strings.Join(lines, "\n")
}

func decodeDeployment(manifest string) (*appsv1.Deployment, error) {
	var dep appsv1.Deployment
	if err := yaml.Unmarshal([]byte(manifest), &dep); err != nil {
		return nil, err
	}
	return &dep, nil
}

func decodeService(manifest string) (*corev1.Service, error) {
	var svc corev1.Service
	if err := yaml.Unmarshal([]byte(manifest), &svc); err != nil {
		return nil, err
	}
	return &svc, nil
}

func firstContainerImage(containers []corev1.Container) string {
	if len(containers) == 0 {
		return "-"
	}
	return containers[0].Image
}

func namespaceOrDefault(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}
