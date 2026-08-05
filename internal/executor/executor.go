package executor

import (
	"context"
	"fmt"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
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
		case planner.OpHelmRepo, planner.OpHelmInstall, planner.OpHelmRepoUpdate, planner.OpHelmUpgrade:
			return fmt.Errorf("executor: use ApplyHelm for helm actions")
		case planner.OpWorkflowCreate:
			return fmt.Errorf("executor: use ApplyArgo for argo workflow actions")
		case planner.OpPipelineRunCreate:
			return fmt.Errorf("executor: use ApplyTekton for tekton actions")
		case planner.OpScaledObjectCreate:
			return fmt.Errorf("executor: use ApplyKEDA for keda actions")
		case planner.OpClaimCreate:
			return fmt.Errorf("executor: use ApplyCrossplane for crossplane actions")
		case planner.OpGitOpsSync:
			return fmt.Errorf("executor: use ApplyGitOpsSync for gitops sync actions")
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
	case "ReplicaSet":
		return r.Client.AppsV1().ReplicaSets(ns).Delete(ctx, name, opts)
	case "Service":
		return r.Client.CoreV1().Services(ns).Delete(ctx, name, opts)
	case "Pod":
		return r.Client.CoreV1().Pods(ns).Delete(ctx, name, opts)
	case "Job":
		return r.Client.BatchV1().Jobs(ns).Delete(ctx, name, opts)
	case "ConfigMap":
		return r.Client.CoreV1().ConfigMaps(ns).Delete(ctx, name, opts)
	case "Secret":
		return r.Client.CoreV1().Secrets(ns).Delete(ctx, name, opts)
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
	case "StatefulSet", "sts":
		return retry.RetryOnConflict(retry.DefaultRetry, func() error {
			sts, err := r.Client.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return err
			}
			sts.Spec.Replicas = &replicas
			_, err = r.Client.AppsV1().StatefulSets(ns).Update(ctx, sts, metav1.UpdateOptions{
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
	case "StatefulSet":
		var sts appsv1.StatefulSet
		if err := yaml.Unmarshal([]byte(a.Manifest), &sts); err != nil {
			return fmt.Errorf("decode StatefulSet: %w", err)
		}
		return r.applyStatefulSet(ctx, &sts)
	case "DaemonSet":
		var ds appsv1.DaemonSet
		if err := yaml.Unmarshal([]byte(a.Manifest), &ds); err != nil {
			return fmt.Errorf("decode DaemonSet: %w", err)
		}
		return r.applyDaemonSet(ctx, &ds)
	case "Service":
		var svc corev1.Service
		if err := yaml.Unmarshal([]byte(a.Manifest), &svc); err != nil {
			return fmt.Errorf("decode Service: %w", err)
		}
		return r.applyService(ctx, &svc)
	case "HorizontalPodAutoscaler":
		var h autoscalingv2.HorizontalPodAutoscaler
		if err := yaml.Unmarshal([]byte(a.Manifest), &h); err != nil {
			return fmt.Errorf("decode HorizontalPodAutoscaler: %w", err)
		}
		return r.applyHPA(ctx, &h)
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

func (r *Runner) applyStatefulSet(ctx context.Context, sts *appsv1.StatefulSet) error {
	ns := sts.Namespace
	if ns == "" {
		ns = "default"
		sts.Namespace = ns
	}
	existing, err := r.Client.AppsV1().StatefulSets(ns).Get(ctx, sts.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = r.Client.AppsV1().StatefulSets(ns).Create(ctx, sts, metav1.CreateOptions{
			FieldManager: FieldManager,
		})
		return err
	}
	if err != nil {
		return err
	}
	sts.ResourceVersion = existing.ResourceVersion
	_, err = r.Client.AppsV1().StatefulSets(ns).Update(ctx, sts, metav1.UpdateOptions{
		FieldManager: FieldManager,
	})
	return err
}

func (r *Runner) applyDaemonSet(ctx context.Context, ds *appsv1.DaemonSet) error {
	ns := ds.Namespace
	if ns == "" {
		ns = "default"
		ds.Namespace = ns
	}
	existing, err := r.Client.AppsV1().DaemonSets(ns).Get(ctx, ds.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = r.Client.AppsV1().DaemonSets(ns).Create(ctx, ds, metav1.CreateOptions{
			FieldManager: FieldManager,
		})
		return err
	}
	if err != nil {
		return err
	}
	ds.ResourceVersion = existing.ResourceVersion
	_, err = r.Client.AppsV1().DaemonSets(ns).Update(ctx, ds, metav1.UpdateOptions{
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

func (r *Runner) applyHPA(ctx context.Context, h *autoscalingv2.HorizontalPodAutoscaler) error {
	ns := h.Namespace
	if ns == "" {
		ns = "default"
		h.Namespace = ns
	}
	existing, err := r.Client.AutoscalingV2().HorizontalPodAutoscalers(ns).Get(ctx, h.Name, metav1.GetOptions{})
	if apierrors.IsNotFound(err) {
		_, err = r.Client.AutoscalingV2().HorizontalPodAutoscalers(ns).Create(ctx, h, metav1.CreateOptions{
			FieldManager: FieldManager,
		})
		return err
	}
	if err != nil {
		return err
	}
	h.ResourceVersion = existing.ResourceVersion
	_, err = r.Client.AutoscalingV2().HorizontalPodAutoscalers(ns).Update(ctx, h, metav1.UpdateOptions{
		FieldManager: FieldManager,
	})
	return err
}
