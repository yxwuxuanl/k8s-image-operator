/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"gomodules.xyz/jsonpatch/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	imagev1 "github.com/yxwuxuanl/k8s-image-operator/api/v1"
)

var (
	cleanFinishedMirrorDuration = flag.Duration("clean-finished-mirror", time.Hour, "clean finished mirror")
)

// MirrorReconciler reconciles a Mirror object
type MirrorReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	JobCreated       = "JobCreated"
	MirrorAnnotation = "image.lin2ur.cn/mirror"
)

//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=mirrors,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=mirrors/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=image.lin2ur.cn,resources=mirrors/finalizers,verbs=update;watch
//+kubebuilder:rbac:groups=batch,resources=jobs,verbs=create;list;get;watch
//+kubebuilder:rbac:groups=core,resources=pods,verbs=list;get;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Mirror object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.17.2/pkg/reconcile
func (r *MirrorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if strings.HasSuffix(req.Name, "-job") {
		req.Name = strings.TrimSuffix(req.Name, "-job")
		if err := r.syncJobStatus(ctx, req); err != nil {
			logger.Error(err, "unable to sync job status")
		}
		return ctrl.Result{}, nil
	}

	if strings.HasSuffix(req.Name, "-pod") {
		req.Name = strings.TrimSuffix(req.Name, "-pod")
		if err := r.syncPodStatus(ctx, req); err != nil {
			logger.Error(err, "unable to sync pod status")
		}
		return ctrl.Result{}, nil
	}

	mirror := &imagev1.Mirror{}
	if err := r.Get(ctx, req.NamespacedName, mirror); err != nil {
		logger.Error(err, "unable to fetch mirror")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	job := buildMirrorJob(mirror)
	_ = ctrl.SetControllerReference(mirror, job, r.Scheme)

	var condition metav1.Condition
	condition.Type = JobCreated
	condition.LastTransitionTime = metav1.NewTime(time.Now())

	err := r.Create(ctx, job)

	if err != nil {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "JobCreateFailed"
		condition.Message = err.Error()
	} else {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "JobCreated"
		condition.Message = "Job created successfully"

		for _, image := range toMirrorImage(mirror.Spec.Images) {
			mirror.Status.Images = append(mirror.Status.Images, imagev1.ImageStatus{
				Source:             image.source,
				Target:             image.target,
				Phase:              "Pending",
				LastTransitionTime: metav1.NewTime(time.Now()),
			})
		}
	}

	meta.SetStatusCondition(&mirror.Status.Conditions, condition)

	_ = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Status().Update(ctx, mirror)
	})

	return ctrl.Result{}, err
}

func (r *MirrorReconciler) syncJobStatus(ctx context.Context, req ctrl.Request) error {
	job := &batchv1.Job{}
	if err := r.Get(ctx, req.NamespacedName, job); err != nil {
		return fmt.Errorf("unable to fetch job: %w", err)
	}

	mirror := &imagev1.Mirror{}
	if err := r.Get(ctx, req.NamespacedName, mirror); err != nil {
		return fmt.Errorf("unable to fetch mirror: %w", err)
	}

	mirror.Status.Running = job.Status.Active
	mirror.Status.Failed = job.Status.Failed
	mirror.Status.Succeeded = job.Status.Succeeded

	for _, condition := range job.Status.Conditions {
		cond := metav1.Condition{
			Type:               "Job" + string(condition.Type),
			Status:             metav1.ConditionStatus(condition.Status),
			LastTransitionTime: condition.LastTransitionTime,
		}

		if reason := condition.Reason; reason != "" {
			cond.Reason = reason
		} else {
			cond.Reason = string(condition.Type)
		}

		if message := condition.Message; message != "" {
			cond.Message = message
		} else {
			cond.Message = "no message"
		}

		meta.SetStatusCondition(&mirror.Status.Conditions, cond)
	}

	_ = retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Client.Status().Update(ctx, mirror)
	})

	return nil
}

func (r *MirrorReconciler) syncPodStatus(ctx context.Context, req ctrl.Request) error {
	pod := &corev1.Pod{}
	if err := r.Get(ctx, req.NamespacedName, pod); err != nil {
		return fmt.Errorf("unable to fetch pod: %w", err)
	}

	var index int
	if v, ok := pod.Labels["batch.kubernetes.io/job-completion-index"]; ok {
		index, _ = strconv.Atoi(v)
	} else {
		return nil
	}

	mirror := &imagev1.Mirror{}
	if err := r.Get(ctx, client.ObjectKey{
		Name:      pod.GetAnnotations()[MirrorAnnotation],
		Namespace: pod.GetNamespace(),
	}, mirror); err != nil {
		return fmt.Errorf("unable to fetch mirror: %w", err)
	}

	image := &mirror.Status.Images[index]
	imageStatus := &imagev1.ImageStatus{
		Source:             image.Source,
		Target:             image.Target,
		Phase:              string(pod.Status.Phase),
		LastTransitionTime: metav1.NewTime(time.Now()),
		Message:            pod.Status.Message,
		Pod:                pod.GetName(),
	}

	jsonPatches, _ := json.Marshal([]jsonpatch.JsonPatchOperation{
		jsonpatch.NewOperation("replace", "/status/images/"+strconv.Itoa(index), imageStatus),
	})

	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		return r.Status().Patch(ctx, mirror, client.RawPatch(types.JSONPatchType, jsonPatches))
	})
}

func (r *MirrorReconciler) cleanFinishedMirror() error {
	var mirrors imagev1.MirrorList
	if err := r.List(context.Background(), &mirrors); err != nil {
		return err
	}

	for _, mirror := range mirrors.Items {
		var lastTransitionTime time.Time

		if cond := meta.FindStatusCondition(mirror.Status.Conditions, "JobComplete"); cond != nil && cond.Status == metav1.ConditionTrue {
			lastTransitionTime = cond.LastTransitionTime.Time
		} else if cond := meta.FindStatusCondition(mirror.Status.Conditions, "JobFailed"); cond != nil && cond.Status == metav1.ConditionTrue {
			lastTransitionTime = cond.LastTransitionTime.Time
		} else {
			continue
		}

		if time.Since(lastTransitionTime) > *cleanFinishedMirrorDuration {
			if err := r.Delete(context.Background(), &mirror); err != nil {
				log.Log.Error(
					err,
					"unable to delete mirror", "mirror", client.ObjectKeyFromObject(&mirror).String(),
				)
				continue
			}

			log.Log.Info("mirror deleted", "mirror", client.ObjectKeyFromObject(&mirror).String())
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MirrorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	if *craneImage == "" {
		return fmt.Errorf("crane-image is required")
	}

	createPred := builder.WithPredicates(predicate.Funcs{
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return meta.FindStatusCondition(
				createEvent.Object.(*imagev1.Mirror).Status.Conditions,
				JobCreated,
			) == nil
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return false
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	})

	updatePred := builder.WithPredicates(predicate.Funcs{
		DeleteFunc: func(event event.DeleteEvent) bool {
			return false
		},
		CreateFunc: func(createEvent event.CreateEvent) bool {
			return false
		},
		UpdateFunc: func(updateEvent event.UpdateEvent) bool {
			return updateEvent.ObjectNew.GetDeletionTimestamp().IsZero()
		},
		GenericFunc: func(genericEvent event.GenericEvent) bool {
			return false
		},
	})

	jobEnqueueFunc := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		for _, reference := range object.GetOwnerReferences() {
			if reference.APIVersion == imagev1.GroupVersion.String() && reference.Kind == "Mirror" {
				return []reconcile.Request{
					{
						NamespacedName: client.ObjectKey{
							Name:      reference.Name + "-job",
							Namespace: object.GetNamespace(),
						},
					},
				}
			}
		}
		return nil
	})

	podEnqueueFunc := handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, object client.Object) []reconcile.Request {
		if anno := object.GetAnnotations(); anno != nil {
			if _, ok := anno[MirrorAnnotation]; ok {
				return []reconcile.Request{
					{
						NamespacedName: client.ObjectKey{
							Name:      object.GetName() + "-pod",
							Namespace: object.GetNamespace(),
						},
					},
				}
			}
		}

		return nil
	})

	go func() {
		t := time.NewTicker(time.Minute)
		for {
			<-t.C
			if err := r.cleanFinishedMirror(); err != nil {
				log.Log.Error(err, "unable to clean finished mirror")
			}
		}
	}()

	return ctrl.NewControllerManagedBy(mgr).
		For(&imagev1.Mirror{}, createPred).
		Watches(&corev1.Pod{}, podEnqueueFunc, updatePred).
		Watches(&batchv1.Job{}, jobEnqueueFunc, updatePred).
		Complete(r)
}
