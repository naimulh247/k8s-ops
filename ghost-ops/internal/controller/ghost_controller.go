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

	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	blogv1 "example.com/api/v1"
)

// GhostReconciler reconciles a Ghost object
type GhostReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=blog.example.com,resources=ghosts,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=blog.example.com,resources=ghosts/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=blog.example.com,resources=ghosts/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Ghost object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.23.1/pkg/reconcile
func (r *GhostReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	log.Info("Reconciling Ghost")

	ghost := &blogv1.Ghost{}
	// get the Ghost resource with the name and namespace specified in the request
	if err := r.Get(ctx, req.NamespacedName, ghost); err != nil {

		log.Error(err, "failed to get ghost")
		return ctrl.Result{}, client.IgnoreNotFound(err)

	}

	log.Info("Reconiling Ghost result:", "imageTag", ghost.Spec.ImageTag, "namespace", ghost.ObjectMeta.Namespace)
	log.Info("Reconciling complete")

	// TODO(user): your logic here

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *GhostReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&blogv1.Ghost{}).
		Named("ghost").
		Complete(r)
}
