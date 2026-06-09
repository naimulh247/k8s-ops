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
	typeAvailableCronJob = "Available"

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

	// status should be able to re-constructed from the state; its not a good idea
	// to read from the status of the root object (cronjob cr). we should reconsturct
	// it on every run

	// find the active list of jobs
	var activeJobs []*kbatch.Job
	var successfulJobs []*kbatch.Job
	var failedJobs []*kbatch.Job
	var mostRecenttime *time.Time // find the last run to update the status

	// a job is considered "finished" if it has "Completed" or "Failed"
	// conditions as true
	isJobFinished := func(job *kbatch.Job) (bool, kbatch.JobConditionType) {
		for _, c := range job.Status.Conditions {
			if (c.Type == kbatch.JobComplete || c.Type == kbatch.JobFailed) && c.Status == corev1.ConditionTrue {
				return true, c.Type
			}
		}
		return false, ""
	}
	// helper to extraact the scheduled time from the job annotation
	getScheduledTimeForJob := func(job *kbatch.Job) (*time.Time, err) {
		timeRaw := job.Annotations[scheduledTimeAnnotation]
		if len(timeRaw) == 0 {
			return nil, nil
		}

		timeParsed, err := time.Parse(time.RFC3339, timeRaw)
		if err != nil {
			return nil, err
		}

		return &timeParsed, nil
	}

	// iterate through the jobs and populate it to the arrays
	for i, job := range childJobs.Items {
		_, finishedType := isJobFinished(&job)

		switch finishedType {
		case "": // still running
			activeJobs = append(activeJobs, &childJobs.Items[i])
		case kbatch.JobFailed:
			failedJobs = append(failedJobs, &childJobs.Items[i])
		case kbatch.JobComplete:
			successfulJobs = append(successfulJobs, &childJobs.Items[i])

		}

		// store the launch time in the annotation, we will rebuild it
		// from the active jobs
		scheduledTimeForJob, err := getScheduledTimeForJob(&job)
		if err != nil {
			log.Error(err, "unable to parse schedule time for child job", "job", &job)
			continue
		}
		if scheduledTimeForJob != nil {
			if mostRecenttime == nil || mostRecenttime.Before(*scheduledTimeForJob) {
				mostRecenttime = scheduledTimeForJob
			}
		}
	}

	if mostRecenttime != nil {
		cronJob.Status.LastScheduleTime = &metav1.Time{Time: *mostRecenttime}
	} else {
		cronJob.Status.LastScheduleTime = nil
	}
	cronJob.Status.Active = nil
	for _, activeJob := range activeJobs {
		// convert the Job onject into a lightweight ObjectReference
		// (stores kind, name, namespace, uid, resourceversion)
		// Job structs fetched from cached so it might have empty fields,
		// pass schema as fallback
		jobRef, err := ref.GetReference(r.Scheme, activeJob)
		if err != nil {
			log.Error(err, "unable to maake a refernece to active job", "job", activeJob)
			continue
		}
		cronJob.Status.Active = append(cronJob.Status.Active, *jobRef)
	}

	log.V(1).Info("job count", "active jobs", len(activeJobs), "successful jobs", len(successfulJobs), "failed jobs", len(failedJobs))

	// check if the CronJob is suspended
	isSuspended := cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend

	// update the status conditions based on the current state
	if isSuspended {
		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeAvailableCronJob,
			Status:  metav1.ConditionFalse,
			Reason:  "Suspended",
			Message: "CronJob is suspended",
		})
	} else if len(failedJobs) > 0 {
		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeDegreadedCronJob,
			Status:  metav1.ConditionTrue,
			Reason:  "JobsFailed",
			Message: fmt.Sprintf("%d job(s) have failed", len(failedJobs)),
		})

		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeAvailableCronJob,
			Status:  metav1.ConditionFalse,
			Reason:  "JobsFailed",
			Message: fmt.Sprintf("%d job(s) have failed", len(failedJobs)),
		})
	} else if len(activeJobs) > 0 {
		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeProgressingCronJob,
			Status:  metav1.ConditionTrue,
			Reason:  "JobsActive",
			Message: fmt.Sprintf("%d job(s) are currently active", len(activeJobs)),
		})

		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeAvailableCronJob,
			Status:  metav1.ConditionTrue,
			Reason:  "JobsActive",
			Message: fmt.Sprintf("%d job(s) are currently active", len(activeJobs)),
		})
	} else {
		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeProgressingCronJob,
			Status:  metav1.ConditionTrue,
			Reason:  "NoJobsActive",
			Message: "No jobs are currently active",
		})

		meta.SetStatusCondition(&cronJob.Status.Conditions, metav1.Condition{
			Type:    typeAvailableCronJob,
			Status:  metav1.ConditionFalse,
			Reason:  "AllJobsCompleted",
			Message: "All jobs have completed succesfully",
		})
	}

	if err := r.Status().Update(ctx, &cronJob); err != nil {
		log.Error(err, "unable to update cronjob status")
		return ctrl.Result{}, err
	}

	// 3 - clean up old jobs based on the history limit
	// deleting is 'best effort' -> if it fails on one,
	// we dont requeue to finish delete
	if cronJob.Spec.FailedJobsHistoryLimit != nil {
		// sort the jobs in place, it goes oldest to newest
		slices.SortStableFunc(failedJobs, func(a, b *kbatch.Job) int {
			aStartTime := a.Status.StartTime
			bStartTime := b.Status.StartTime
			if aStartTime == nil && bStartTime != nil {
				return 1
			}

			if aStartTime.Before(bStartTime) {
				return -1
			} else if bStartTime.Before(aStartTime) {
				return 1
			}
			return 0
		})

		for i, job := range failedJobs {
			// check if we need to delete the older jobs based on the limit (sorted old to latest)
			if i >= len(failedJobs)-int(*cronJob.Spec.FailedJobsHistoryLimit) {
				break
			}
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); client.IgnoreNotFound(err) != nil {
				log.Error(err, "unabled to delete old failed job", "job", job)
			} else {
				log.V(1).Info("deleted old failed job", "job", job)
			}
		}
	}

	if cronJob.Spec.SuccessfulJobsHistoryLimit != nil {
		// sort the succesful job arry in oldest to latest
		slices.SortStableFunc(successfulJobs, func(a, b *kbatch.Job) int {
			aStartTime := a.Status.StartTime
			bStartTime := b.Status.StartTime

			if aStartTime == nil && bStartTime != nil {
				return 1
			}

			if aStartTime.Before(bStartTime) {
				return -1
			} else if bStartTime.Before(aStartTime) {
				return 1
			}
			return 0
		})

		for i, job := range successfulJobs {
			if i >= len(successfulJobs)-int(*cronJob.Spec.SuccessfulJobsHistoryLimit) {
				break
			}
			if err := r.Delete(ctx, job, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil {
				log.Error(err, "unable to delete old successful job", "job", job)

			} else {
				log.V(1).Info("deleted old succesful job", "job", job)
			}
		}
	}

	// 4 - check if suspended
	// dont run any jobs / stop for now
	if cronJob.Spec.Suspend != nil && *cronJob.Spec.Suspend {
		log.V(1).Info("cronjob suspended, skipping")
		return ctrl.Result{}, nil
	}

	// 5 - get the next scheduled run

	getNextSchedule := func(cronJob *batchv1alpha1.CronJob, now time.Time) (lastMissed time.Time, next time.Time, err error) {
		sched, err := cron.ParseStandard(cronJob.Spec.Schedule)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("unparsable scheduled %q: %w", cronJob.Spec.Schedule, err)
		}
		// start from the last observed run tme
		var earliestTime time.Time
		if cronJob.Status.LastScheduleTime != nil { // when we last successfuly ran
			earliestTime = cronJob.Status.LastScheduleTime.Time
		} else {
			earliestTime = cronJob.CreationTimestamp.Time // or look at creation time if we never ran successfully
		}

		// we only care about the missed runs in the last StartingDealineSeconds
		if cronJob.Spec.StartingDeadlineSeconds != nil {
			scheduleDeadline := now.Add(-time.Second * time.Duration(*cronJob.Spec.StartingDeadlineSeconds))

			if scheduleDeadline.After(earliestTime) {
				earliestTime = scheduleDeadline
			}
		}
		if earliestTime.After(now) {
			return time.Time{}, sched.Next(now), nil
		}

		// check for missed runs between the earlies time and now at the interval of sched
		starts := 0
		for t := sched.Next(earliestTime); !t.After(now); t = sched.Next(t) {
			lastMissed = t

			starts++

			if starts > 100 {
				// cant get the most recont times
				return time.Time{}, time.Time{}, fmt.Errorf("Too many missed start time (> 100). Set or decrease .spec.startingDeadlineSeconds or check clock skew")
			}
		}
		return lastMissed, sched.Next(now), nil
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
