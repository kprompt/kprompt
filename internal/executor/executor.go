package executor

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/yaml"

	"github.com/kprompt/kprompt/internal/planner"
)

const FieldManager = "kprompt"

// Runner applies approved plans to the cluster.
type Runner struct {
	Client kubernetes.Interface
}

// Apply executes mutating actions (scale, deploy, rollback, delete).
func (r *Runner) Apply(ctx context.Context, plan planner.ExecutionPlan) error {
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpScale:
			if err := r.scale(ctx, a); err != nil {
				return err
			}
		case planner.OpRollback:
			if err := r.rollback(ctx, a); err != nil {
				return err
			}
		case planner.OpDelete:
			if err := r.delete(ctx, a); err != nil {
				return err
			}
		case planner.OpCreate, planner.OpUpdate:
			if err := r.applyManifest(ctx, a); err != nil {
				return err
			}
		case planner.OpHelmRepo, planner.OpHelmInstall:
			return fmt.Errorf("executor: use ApplyHelm for helm actions")
		default:
			return fmt.Errorf("executor: unsupported op %q", a.Op)
		}
	}
	return nil
}

func (r *Runner) delete(ctx context.Context, a planner.Action) error {
	name := strings.TrimSpace(a.Object.Name)
	if name == "" {
		return fmt.Errorf("delete missing object name")
	}
	ns := a.Object.Namespace
	if ns == "" {
		ns = "default"
	}
	policy := metav1.DeletePropagationBackground
	opts := metav1.DeleteOptions{PropagationPolicy: &policy}
	switch a.Object.Kind {
	case "Deployment":
		return r.Client.AppsV1().Deployments(ns).Delete(ctx, name, opts)
	case "Service":
		return r.Client.CoreV1().Services(ns).Delete(ctx, name, opts)
	case "Pod":
		return r.Client.CoreV1().Pods(ns).Delete(ctx, name, opts)
	default:
		return fmt.Errorf("delete of %s not implemented", a.Object.Kind)
	}
}

func (r *Runner) rollback(ctx context.Context, a planner.Action) error {
	ns := a.Object.Namespace
	if ns == "" {
		ns = "default"
	}
	toRev := int64(0)
	if a.Revision != nil {
		toRev = *a.Revision
	}
	switch a.Object.Kind {
	case "Deployment", "":
		return rollbackDeployment(ctx, r.Client, ns, a.Object.Name, toRev)
	default:
		return fmt.Errorf("rollback of %s not implemented", a.Object.Kind)
	}
}

func (r *Runner) scale(ctx context.Context, a planner.Action) error {
	if a.Replicas == nil {
		return fmt.Errorf("scale action missing replicas")
	}
	ns := a.Object.Namespace
	if ns == "" {
		ns = "default"
	}
	name := a.Object.Name
	replicas := *a.Replicas

	switch a.Object.Kind {
	case "Deployment", "":
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			dep, err := r.Client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			dep.Spec.Replicas = &replicas
			_, err = r.Client.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{
				FieldManager: FieldManager,
			})
			return err
		})
	default:
		return fmt.Errorf("scale of %s not implemented", a.Object.Kind)
	}
}

func (r *Runner) applyManifest(ctx context.Context, a planner.Action) error {
	if strings.TrimSpace(a.Manifest) == "" {
		return fmt.Errorf("create/update action missing manifest")
	}
	switch a.Object.Kind {
	case "Deployment":
		var dep appsv1.Deployment
		if err := yaml.Unmarshal([]byte(a.Manifest), &dep); err != nil {
			return fmt.Errorf("decode Deployment: %w", err)
		}
		return r.applyDeployment(ctx, &dep)
	case "Service":
		var svc corev1.Service
		if err := yaml.Unmarshal([]byte(a.Manifest), &svc); err != nil {
			return fmt.Errorf("decode Service: %w", err)
		}
		return r.applyService(ctx, &svc)
	default:
		return fmt.Errorf("apply of %s not implemented", a.Object.Kind)
	}
}

func (r *Runner) applyDeployment(ctx context.Context, dep *appsv1.Deployment) error {
	ns := dep.Namespace
	if ns == "" {
		ns = "default"
		dep.Namespace = ns
	}
	existing, err := r.Client.AppsV1().Deployments(ns).Get(ctx, dep.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = r.Client.AppsV1().Deployments(ns).Create(ctx, dep, metav1.CreateOptions{
			FieldManager: FieldManager,
		})
		return err
	}
	if err != nil {
		return err
	}
	dep.ResourceVersion = existing.ResourceVersion
	_, err = r.Client.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{
		FieldManager: FieldManager,
	})
	return err
}

func (r *Runner) applyService(ctx context.Context, svc *corev1.Service) error {
	ns := svc.Namespace
	if ns == "" {
		ns = "default"
		svc.Namespace = ns
	}
	existing, err := r.Client.CoreV1().Services(ns).Get(ctx, svc.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = r.Client.CoreV1().Services(ns).Create(ctx, svc, metav1.CreateOptions{
			FieldManager: FieldManager,
		})
		return err
	}
	if err != nil {
		return err
	}
	svc.ResourceVersion = existing.ResourceVersion
	svc.Spec.ClusterIP = existing.Spec.ClusterIP
	svc.Spec.ClusterIPs = existing.Spec.ClusterIPs
	_, err = r.Client.CoreV1().Services(ns).Update(ctx, svc, metav1.UpdateOptions{
		FieldManager: FieldManager,
	})
	return err
}
