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
	batchv1 "k8s.io/api/batch/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// CronJobSpec defines the desired state of CronJob
type CronJobSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	// The following markers will use OpenAPI v3 schema to validate the value
	// More info: https://book.kubebuilder.io/reference/markers/crd-validation.html

	// schedule in cron format (ex, "0 0 * * *")
	// +kubebuilder:validation:MinLength=0
	// +required
	Schedule string `json:"schedule"`

	// deine in seconds for starting the job if it misses its scheduled time
	// missed job will count as failed job
	// +optional
	// +kubebuilder:validation:Minimum=0
	StartingDeadlineSeconds *int64 `json:"startingDeadlineSeconds,omitempty"`

	// concurrencyPolicy - specifies how to treat concurrent executions of a Job
	// Valid optoins are:
	// - "Allow" (default): allows CronJobs to run concurrently
	// - "Forbid": forbids concurrent runs, skipping next run if previous run didnt finish yet
	// - "Replace": cancels currrently running job and replaces with new one
	//  +optional
	// +kubebuilder:default:=Allow
	ConcurrencyPolicy ConcurrencyPolicy `json:"concurrencyPolicy,omitempty"`

	// suspends tells the controller to suspend the next scheduled run, 
	// it doesnt apply to already running jobs, but prevents new ones from being created
	// +optional
	Suspend *bool `json:"suspend,omitempty"`

	// define the job that will be created when executing the cronjob
	JobTemplate batchv1.JobTemplateSpec `json:"jobTemplate"`

	// defines the number of successful finished jobs to be retained
	// +optional
	// +kubebuilder:validation:Minimum=0
	SuccessfulJobsHistoryLimit *int32 `json:"successfulJobsHistoryLimit,omitempty"`

	// defines the number of failed finished jobs to be retained
	// using a pointer to distinguish between explicit zero and not specified
	// +optional
	// +kubebuilder:validation:Minimum=0
	FailedJobsHistoryLimit *int32 `json:"failedJobsHistoryLimit,omitempty"`


}

// CronJobStatus defines the observed state of CronJob.
type CronJobStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// For Kubernetes API conventions, see:
	// https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#typical-status-properties

	// conditions represent the current state of the CronJob resource.
	// Each condition has a unique type and reflects the status of a specific aspect of the resource.
	//
	// Standard condition types include:
	// - "Available": the resource is fully functional
	// - "Progressing": the resource is being created or updated
	// - "Degraded": the resource failed to reach or maintain its desired state
	//
	// The status of each condition is one of True, False, or Unknown.
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// CronJob is the Schema for the cronjobs API
type CronJob struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is a standard object metadata
	// +optional
	metav1.ObjectMeta `json:"metadata,omitzero"`

	// spec defines the desired state of CronJob
	// +required
	Spec CronJobSpec `json:"spec"`

	// status defines the observed state of CronJob
	// +optional
	Status CronJobStatus `json:"status,omitzero"`
}

// +kubebuilder:object:root=true

// CronJobList contains a list of CronJob
type CronJobList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitzero"`
	Items           []CronJob `json:"items"`
}

func init() {
	SchemeBuilder.Register(&CronJob{}, &CronJobList{})
}
