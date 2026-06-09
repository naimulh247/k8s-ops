/*
Copyright 2026.

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
	"fmt"
	"maps"
	"slices"
	"time"

	"github.com/robfig/cron"
	kbatch "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ref "k8s.io/client-go/tools/reference"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	batchv1alpha1 "nai-k8s-ops.com/cronjob/api/v1alpha1"
)

// CronJobReconciler reconciles a CronJob object
type CronJobReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// clock allows use to fake timing
	Clock
}

type realClock struct{}

func (_ realClock) Now() time.Time { return time.Now() }

// Clock knows how to get the current time
type Clock interface {
	Now() time.Time
}

// CronJob status definitions
const (
	// typeAvailable - the status of the CronJob reconciliation
	typeAvailable = "Available"

	// typeProgressingCronJob - the statuse used when CronJob is being reconciled
	typeProgressingCronJob = "Progressing"

	// typeDegradedCronJob - the status used when the CronJob encountered a error
	typeDegreadedCronJob = "Degraded"
)

// scheduledTimeAnnotation is the annotation key used to stamp the scheduled
// time onto each Job we create. This allows us to recover lastScheduledTime
// by reading the Job directly, rather than relying on our own status fied
var (
	scheduledTimeAnnotation = "batch.nai-k8s-ops.com/cronjob/scheduled-at"
)

// +kubebuilder:rbac:groups=batch.nai-k8s-ops.com,resources=cronjobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch.nai-k8s-ops.com,resources=cronjobs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=batch.nai-k8s-ops.com,resources=cronjobs/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=batch,resources=jobs/status,verbs=get
// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the CronJob object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *CronJobReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	// fetch the CronJob instance; this will get populated with the data from the cluster
	var cronJob batchv1alpha1.CronJob
	if err := r.Get(ctx, req.NamespacedName, &cronJob); err != nil {
		if apierrors.IsNotFound(err) {
			// if the cr is not found, it might have been deleted or not created
			// so we will not requeue and return nil
			log.Info("CronJob resource not found. Ignoring since object must be deleted or not created yet.")
			return ctrl.Result{}, nil
		}

		// error reading the object - requeue the request
		log.Error(err, "Failed to get CronJob")
		return ctrl.Result{}, err
	}

	// intialize the status confitions if not yet present
	if len(cronJob.Status.Conditions) == 0 {
		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeProgressingCronJob,
			Status:  metav1.ConditionUnknown,
			Reason:  "Reconciling",
			Message: "Starting Reconciliation",
		})

		if err := r.Status().Update(ctx, &cronJob); err != nil {
			log.Error(err, "Failed to update CronJob status")
			return ctrl.Result{}, err
		}

		// kubernetes uses optimistic concurrency - any updates (including the status updates)
		// changes the resource version. if we continue to reconcile, following updates may conflict
		//  to keep reconciliation logic in sycn with the actual cluster state, refetch the latest data
		if err := r.Get(ctx, req.NamespacedName, &cronJob); err != nil {
			log.Error(err, "Failed to re-fetch CronJob")
			return ctrl.Result{}, err
		}

	}

	// 2: list all active jobs, and update the status
	// to update our status, we need to get all child jobs in the current namespace that belong to cronjob
	var childJobs kbatch.JobList
	// we use the List menthod to get the list of all child jobs
	// jobOwnerKey is an index for the controller to be able to find the owned jobs faster
	if err := r.List(ctx, &childJobs, client.InNamespace(req.Namespace), client.MatchingFields{jobOwnerKey: req.Name}); err != nil {
		log.Error(err, "unable to list child Jobs")

		// before updating the check we have the latest state
		if fetchErr := r.Get(ctx, req.NamespacedName, &cronJob); err != nil {
			log.Error(fetchErr, "failed to re-fetch cronjob")
			return ctrl.Result{}, fetchErr
		}
		// update the state condition for err
		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeDegreadedCronJob,
			Status:  metav1.ConditionTrue,
			Reason:  "ReconciliationError",
			Message: fmt.Sprintf("Failed to list child jobs: %v", err),
		})

		if statusErr := r.Status().Update(ctx, &cronJob); statusErr != nil {
			log.Error(statusErr, "Failed to updated cronjob status")
		}
		return ctrl.Result{}, err

	}


	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *CronJobReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&batchv1alpha1.CronJob{}).
		Named("cronjob").
		Complete(r)
}
