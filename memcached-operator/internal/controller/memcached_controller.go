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
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	cachev1alpha1 "nai-k8s-ops.com/memcached/api/v1alpha1"
)

// MemcachedReconciler reconciles a Memcached object
type MemcachedReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=cache.nai-k8s-ops.com,resources=memcacheds,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cache.nai-k8s-ops.com,resources=memcacheds/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cache.nai-k8s-ops.com,resources=memcacheds/finalizers,verbs=update
// +kubebuilder:rbac:groups=events.k8s.io,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Memcached object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *MemcachedReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// get the logger from the context
	logger := logf.FromContext(ctx)

	// get the Memcached instance
	// check if the cr for the Memcached Kind is appiled on the cluster,
	// if not return nil and stop the reconciliation
	memchached := &cachev1alpha1.Memcached{}
	err := r.Get(ctx, req.NamespacedName, memchached)
	if err != nil {
		// if the customer resource is not found, it means it hasnt been created or deleted
		// so we can ignore the error and stop the reconciliation
		if apierrors.IsNotFound(err) {
			logger.Info("Memcached resource not found. Ignoring since the object might be deleted")
			return ctrl.Result{}, nil
		}

		// for any other error, reading the object, requeue the request to try again
		// it knows to requeue the request because of the error, so we can return an empty result and the error
		logger.Error(err, "Failed to get Memcached")
		return ctrl.Result{}, err
	}

	// if the status conditions is empty, it means this is the first time we are reconciling this object, so we can set the initial status condition to unknown and update the status on the api server
	if len(memchached.Status.Conditions) == 0 {
		// modify the local struct in memory
		meta.SetStatusCondition(&memchached.Status.Conditions, metav1.Condition{Type: "Available", Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reocniliation"})
		// update the status on the api server
		if err = r.Status().Update(ctx, memchached); err != nil {
			logger.Error(err, "Failed to update Memcached status")
			return ctrl.Result{}, err
		}
	}

	// refresh the memecached cr after the status update,
	// this will prevent modified object error while we handle the cr
	if err = r.Get(ctx, req.NamespacedName, memchached); err != nil {
		logger.Error(err, "Failed to re-fetch memcached")
		return ctrl.Result{}, err
	}

	// check if the deployment for memcached exists, if not we will create a new one
	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Namespace: memchached.Namespace, Name: memchached.Name}, foundDeployment)
	if err != nil && apierrors.IsNotFound(err) {
		// define a new deployment
		dep, err := r.deploymentForMemcached(memchached) // this function is not implemented yet, we will implement it later
		if err != nil {
			logger.Error(err, "Failed to define new deployment for Memcached")

			// update the status
			meta.SetStatusCondition(&memchached.Status.Conditions,
				metav1.Condition{Type: "Available",
					Status:  metav1.ConditionFalse,
					Reason:  "Reconciling",
					Message: fmt.Sprintf("Failed to create deployment for custom resource (%s): (%s)", memchached.Name, err)})
			
			if err := r.Status().Update(ctx, memchached); err != nil {
				logger.Error(err, "Failed to update Memcached status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}

		logger.Info("Creating a new deployment",
			"Deployment.Namespace", dep.Namespace, "Deployemnt.Name", dep.Name)
		if err = r.Create(ctx, dep); err != nil {
			logger.Error(err, "Failed to create a new deployment", "Deployment.Namespace", dep.Namespace, "Deplyment.Name", dep.Name)
			return ctrl.Result{}, err
		}

		// at this point deployment is created succesfully. we requeue the reconciliation to chekc the state and move to next step
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment")
		// return the error for reconcilation to re-try
		return ctrl.Result{}, err
	}

	// at this point we know the deployment exists, 
	// we can check the size in the cr/ spec and compare it with the actual size
	var desiredReplicas int32 = 0
	if memchached.Spec.Size != nil {
		desiredReplicas = *memchached.Spec.Size
	}

	// the crd api defines that the memcached size field (MemcachedSpec.Size)
	// to set the number of deployment instances to the desired size / state on the cluster
	// check if the CR size is defined and if it is different from the actual deployment size, 
	// then update the deployment with the desired size

	if foundDeployment.Spec.Replicas == nil || *foundDeployment.Spec.Replicas != desiredReplicas {
		foundDeployment.Spec.Replicas = ptr.To(desiredReplicas) // this is a helper function from k8s utils to get a pointer to the desired replicas value
		if err = r.Update(ctx, foundDeployment); err != nil {
			logger.Error(err, "Failed to update deployment", "Deployment.Namespace", foundDeployment.Namespace, "Deployment.Name", foundDeployment.Name)
			
			// re-fetch the memcached cr to get the lates version of the object
			if err := r.Get(ctx, req.NamespacedName, memchached); err != nil {
				logger.Error(err, "Failed to re-fetch memcached")
				return ctrl.Result{}, err
			}

			// update the status 
			meta.SetStatusCondition(&memchached.Status.Conditions, 
				metav1.Condition{
					Type: "Available",
					Status: metav1.ConditionFalse,
					Reason: "Resizing",
					Message: fmt.Sprintf("Failed to update the size for cusom resource (%s): (%s)", memchached.Name, err),
				})
			
			if err := r.Status().Update(ctx, memchached); err != nil {
				logger.Error(err, "Failed to update Memcached status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err

		}

		// at this pint we know the deployment is updated with the desired size, 
		// we should requie the reconciliation to have the latest state of the resouece before updating
		return ctrl.Result{RequeueAfter: time.Minute}, nil

	}
	// this is to update the status or reocnciling of custom resource with n replicas
	meta.SetStatusCondition(&memchached.Status.Conditions, metav1.Condition{
		Type: "Availble",
		Status: metav1.ConditionTrue,
		Reason: "Reconciling",
		Message: fmt.Sprintf("Deploymet of custom resource (%s) with %d replicas created successfully", memchached.Name, desiredReplicas),
	})
	if err := r.Status().Update(ctx, memchached); err != nil {
		logger.Error(err, "Failed to update Memcached status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// deploymentForMemcached returns a memcached Deployment object
func (r *MemcachedReconciler) deploymentForMemcached(memchached *cachev1alpha1.Memcached) (*appsv1.Deployment, error) {
	// memcahced image
	image := "memcached:latest"

	// define the deployment object
	dep := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      memchached.Name,
            Namespace: memchached.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            Replicas: memchached.Spec.Size,
            Selector: &metav1.LabelSelector{
                MatchLabels: map[string]string{"app.kubernetes.io/name": "project"},
            },
            Template: corev1.PodTemplateSpec{
                ObjectMeta: metav1.ObjectMeta{
                    Labels: map[string]string{"app.kubernetes.io/name": "project"},
                },
                Spec: corev1.PodSpec{
                    SecurityContext: &corev1.PodSecurityContext{
                        RunAsNonRoot: ptr.To(true),
                        SeccompProfile: &corev1.SeccompProfile{
                            Type: corev1.SeccompProfileTypeRuntimeDefault,
                        },
                    },
                    Containers: []corev1.Container{{
                        Image:           image,
                        Name:            "memcached",
                        ImagePullPolicy: corev1.PullIfNotPresent,
                        SecurityContext: &corev1.SecurityContext{
                            RunAsNonRoot:             ptr.To(true),
                            RunAsUser:                ptr.To(int64(1001)),
                            AllowPrivilegeEscalation: ptr.To(false),
                            Capabilities: &corev1.Capabilities{
                                Drop: []corev1.Capability{
                                    "ALL",
                                },
                            },
                        },
                        Ports: []corev1.ContainerPort{{
                            ContainerPort: 11211,
                            Name:          "memcached",
                        }},
                        Command: []string{"memcached", "--memory-limit=64", "-o", "modern", "-v"},
                    }},
                },
            },
        },
    }

	// set the ower reference for the deployment to be the memcached cr, this will help with garbage collection and also with watching the deployment for changes
	if err := ctrl.SetControllerReference(memchached, dep, r.Scheme); err != nil {
		return nil, err
	}

	return dep, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// watch the memcached cr for changes and reconcile when it is created, updated or deleted
		For(&cachev1alpha1.Memcached{}).
		Named("memcached").
		// whatch the deployment managed by the memcached controller. 
		// if something changes owned by the memcached controler, it will trigger 
		// reconiliation 
		Owns(&appsv1.Deployment{}).
		Complete(r)
}
