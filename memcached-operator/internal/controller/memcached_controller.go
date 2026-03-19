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

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
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
		meta.SetStatusCondition(&memchached.Status.Conditions, metav1.Condition{Type: "Availble", Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reocniliation"})
		// update the status on the api server
		if err = r.Status().Update(ctx, memchached); err != nil {
			logger.Error(err, "Failed to update Memcached status")
			return ctrl.Result{}, err
		}
	}

	

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MemcachedReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&cachev1alpha1.Memcached{}).
		Named("memcached").
		Complete(r)
}
