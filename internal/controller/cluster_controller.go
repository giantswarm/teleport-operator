/*
Copyright 2023.

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
	"time"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	Client   client.Client
	Log      logr.Logger
	Scheme   *runtime.Scheme
	Teleport *teleport.Teleport
}

//+kubebuilder:rbac:groups=cluster.x-k8s.io.giantswarm.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io.giantswarm.io,resources=clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.x-k8s.io.giantswarm.io,resources=clusters/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Cluster object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.14.4/pkg/reconcile
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	r.Teleport.Logger = r.Log.WithValues("cluster", req.NamespacedName)
	r.Teleport.CtrlClient = r.Client

	cluster := &capi.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, microerror.Mask(err)
	}

	// Check if the cluster instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	// if it is, delete the cluster from teleport
	if !cluster.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.Teleport.EnsureClusterDeregistered(ctx, cluster)
	}

	// Register teleport for MC/WC
	if err := r.Teleport.EnsureClusterRegistered(ctx, cluster); err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	// We need to constantly requeue to check the token validity
	// and re-generate and update secret for the cluster
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}
