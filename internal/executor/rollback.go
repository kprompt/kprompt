package executor

import (
	"context"
	"fmt"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

const revisionAnnotation = "deployment.kubernetes.io/revision"

// rollbackDeployment undoes a Deployment rollout to toRevision (0 = previous).
func rollbackDeployment(ctx context.Context, client kubernetes.Interface, ns, name string, toRevision int64) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		dep, err := client.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			return err
		}
		rsList, err := client.AppsV1().ReplicaSets(ns).List(ctx, metav1.ListOptions{})
		if err != nil {
			return err
		}
		owned := ownedReplicaSets(dep, rsList.Items)
		if len(owned) == 0 {
			return fmt.Errorf("no ReplicaSets found for Deployment/%s", name)
		}
		target, err := findRollbackRS(owned, dep, toRevision)
		if err != nil {
			return err
		}
		applyRSTemplate(dep, target)
		_, err = client.AppsV1().Deployments(ns).Update(ctx, dep, metav1.UpdateOptions{
			FieldManager: FieldManager,
		})
		return err
	})
}

func ownedReplicaSets(dep *appsv1.Deployment, items []appsv1.ReplicaSet) []appsv1.ReplicaSet {
	out := make([]appsv1.ReplicaSet, 0, len(items))
	for i := range items {
		rs := items[i]
		if !metav1.IsControlledBy(&rs, dep) {
			continue
		}
		out = append(out, rs)
	}
	return out
}

func findRollbackRS(owned []appsv1.ReplicaSet, dep *appsv1.Deployment, toRevision int64) (*appsv1.ReplicaSet, error) {
	current := revisionOf(&dep.ObjectMeta)
	if toRevision == 0 {
		var best *appsv1.ReplicaSet
		var bestRev int64 = -1
		for i := range owned {
			rs := &owned[i]
			rev := revisionOf(&rs.ObjectMeta)
			if rev > 0 && rev < current && rev > bestRev {
				bestRev = rev
				best = rs
			}
		}
		if best == nil {
			return nil, fmt.Errorf("no previous revision to roll back to (current revision %d)", current)
		}
		return best, nil
	}
	for i := range owned {
		rs := &owned[i]
		if revisionOf(&rs.ObjectMeta) == toRevision {
			if toRevision == current {
				return nil, fmt.Errorf("Deployment/%s is already at revision %d", dep.Name, toRevision)
			}
			return rs, nil
		}
	}
	return nil, fmt.Errorf("revision %d not found for Deployment/%s", toRevision, dep.Name)
}

func revisionOf(meta *metav1.ObjectMeta) int64 {
	if meta == nil || meta.Annotations == nil {
		return 0
	}
	s := meta.Annotations[revisionAnnotation]
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0
	}
	return n
}

func applyRSTemplate(dep *appsv1.Deployment, rs *appsv1.ReplicaSet) {
	dep.Spec.Template = *rs.Spec.Template.DeepCopy()
	if dep.Spec.Template.Labels != nil {
		delete(dep.Spec.Template.Labels, appsv1.DefaultDeploymentUniqueLabelKey)
	}
}
