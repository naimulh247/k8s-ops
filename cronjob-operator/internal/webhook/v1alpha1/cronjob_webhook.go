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

package v1alpha1

import (
	"context"

	"github.com/robfig/cron"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	batchv1alpha1 "nai-k8s-ops.com/cronjob/api/v1alpha1"
)

// nolint:unused
// log is for logging in this package.
var cronjoblog = logf.Log.WithName("cronjob-resource")

// SetupCronJobWebhookWithManager registers the webhook for CronJob in the manager.
func SetupCronJobWebhookWithManager(mgr ctrl.Manager) error {
	return ctrl.NewWebhookManagedBy(mgr, &batchv1alpha1.CronJob{}).
		WithValidator(&CronJobCustomValidator{}).
		WithDefaulter(&CronJobCustomDefaulter{
			DefaultConcurrencyPolicy:          batchv1alpha1.AllowConcurrent,
			DefaultSuspend:                    false,
			DefaultSuccessfulJobsHistoryLimit: 3,
			DefaultFailedJobsHistoryLimit:     1,
		}).
		Complete()
}

// TODO(user): EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!

// +kubebuilder:webhook:path=/mutate-batch-nai-k8s-ops-com-v1alpha1-cronjob,mutating=true,failurePolicy=fail,sideEffects=None,groups=batch.nai-k8s-ops.com,resources=cronjobs,verbs=create;update,versions=v1alpha1,name=mcronjob-v1alpha1.kb.io,admissionReviewVersions=v1

// CronJobCustomDefaulter struct is responsible for setting default values on the custom resource of the
// Kind CronJob when those are created or updated.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as it is used only for temporary operations and does not need to be deeply copied.
type CronJobCustomDefaulter struct {
	// TODO(user): Add more fields as needed for defaulting

	// default values for CronJob fields
	DefaultConcurrencyPolicy          batchv1alpha1.ConcurrencyPolicy
	DefaultSuspend                    bool
	DefaultSuccessfulJobsHistoryLimit int32
	DefaultFailedJobsHistoryLimit     int32
}

// Default implements webhook.CustomDefaulter so a webhook will be registered for the Kind CronJob.
func (d *CronJobCustomDefaulter) Default(_ context.Context, obj *batchv1alpha1.CronJob) error {
	cronjoblog.Info("Defaulting for CronJob", "name", obj.GetName())

	// TODO(user): fill in your defaulting logic.

	// set default valudes
	d.applyDefaults(obj)

	return nil
}

// applyDefaults applies default values to CronJob CR fields
func (d *CronJobCustomDefaulter) applyDefaults(cronJob *batchv1alpha1.CronJob) {
	if cronJob.Spec.ConcurrencyPolicy == "" {
		cronJob.Spec.ConcurrencyPolicy = d.DefaultConcurrencyPolicy
	}
	if cronJob.Spec.Suspend == nil {
		cronJob.Spec.Suspend = &d.DefaultSuspend
	}
	if cronJob.Spec.SuccessfulJobsHistoryLimit == nil {
		cronJob.Spec.SuccessfulJobsHistoryLimit = &d.DefaultSuccessfulJobsHistoryLimit
	}
	if cronJob.Spec.FailedJobsHistoryLimit == nil {
		cronJob.Spec.FailedJobsHistoryLimit = &d.DefaultFailedJobsHistoryLimit
	}
}

// TODO(user): change verbs to "verbs=create;update;delete" if you want to enable deletion validation.
// NOTE: If you want to customise the 'path', use the flags '--defaulting-path' or '--validation-path'.
// +kubebuilder:webhook:path=/validate-batch-nai-k8s-ops-com-v1alpha1-cronjob,mutating=false,failurePolicy=fail,sideEffects=None,groups=batch.nai-k8s-ops.com,resources=cronjobs,verbs=create;update,versions=v1alpha1,name=vcronjob-v1alpha1.kb.io,admissionReviewVersions=v1

// CronJobCustomValidator struct is responsible for validating the CronJob resource
// when it is created, updated, or deleted.
//
// NOTE: The +kubebuilder:object:generate=false marker prevents controller-gen from generating DeepCopy methods,
// as this struct is used only for temporary operations and does not need to be deeply copied.
type CronJobCustomValidator struct {
	// TODO(user): Add more fields as needed for validation
}

// for the sake of simplicity validate create and update will behavce the same and do nothring during deletion
func validateCronJob(cronJob *batchv1alpha1.CronJob) error {
	var allErrs field.ErrorList
	if err := validateCronJobName(cronJob); err != nil {
		allErrs = append(allErrs, err)
	}
	if err := validateCronJobSpec(cronJob); err != nil {
		allErrs = append(allErrs, err)
	}

	// return all the errors from the validation
	return apierrors.NewInvalid(
		schema.GroupKind{Group: "batch.nai-k8s-ops.com", Kind: "CronJob"}, cronJob.Name, allErrs)

}

func validateCronJobSpec(cronJob *batchv1alpha1.CronJob) *field.Error {
	if _, err := cron.ParseStandard(cronJob.Spec.Schedule); err != nil {
		return field.Invalid(field.NewPath("spec").Child("schedule"), cronJob.Spec.Schedule, err.Error())
	}
	return nil
}

func validateCronJobName(cronJob *batchv1alpha1.CronJob) *field.Error {
	// k8s object names needs to fit in 63 chraracter limit, we need to make sure that 
	// cronjob object name is at max <= 52
	if len(cronJob.Name) > validation.DNS1123SubdomainMaxLength - 11 {
		return field.Invalid(field.NewPath("metadata").Child("name"), cronJob.Name, "must be no more thna 52 characters")
	}

	return nil
}

// ValidateCreate implements webhook.CustomValidator so a webhook will be registered for the type CronJob.
func (v *CronJobCustomValidator) ValidateCreate(_ context.Context, obj *batchv1alpha1.CronJob) (admission.Warnings, error) {
	cronjoblog.Info("Validation for CronJob upon creation", "name", obj.GetName())

	return nil, nil
}

// ValidateUpdate implements webhook.CustomValidator so a webhook will be registered for the type CronJob.
func (v *CronJobCustomValidator) ValidateUpdate(_ context.Context, oldObj, newObj *batchv1alpha1.CronJob) (admission.Warnings, error) {
	cronjoblog.Info("Validation for CronJob upon update", "name", newObj.GetName())

	// TODO(user): fill in your validation logic upon object update.

	return nil, nil
}

// ValidateDelete implements webhook.CustomValidator so a webhook will be registered for the type CronJob.
func (v *CronJobCustomValidator) ValidateDelete(_ context.Context, obj *batchv1alpha1.CronJob) (admission.Warnings, error) {
	cronjoblog.Info("Validation for CronJob upon deletion", "name", obj.GetName())

	// TODO(user): fill in your validation logic upon object deletion.

	return nil, nil
}
