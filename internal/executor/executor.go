package executor

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"

	"github.com/kprompt/kprompt/internal/planner"
)

const FieldManager = "kprompt"

// Runner applies approved plans to the cluster.
type Runner struct {
	Client kubernetes.Interface
}

// Apply executes mutating actions. v0 supports Deployment scale.
func (r *Runner) Apply(ctx context.Context, plan planner.ExecutionPlan) error {
	for _, a := range plan.Actions {
		switch a.Op {
		case planner.OpScale:
			if err := r.scale(ctx, a); err != nil {
				return err
			}
		default:
			return fmt.Errorf("executor: unsupported op %q in v0", a.Op)
		}
	}
	return nil
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
		return fmt.Errorf("scale of %s not implemented in v0", a.Object.Kind)
	}
}
