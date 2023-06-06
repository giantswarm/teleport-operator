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
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/pkg/teleportclient"

	"github.com/gravitational/teleport/api/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	TeleportClient *teleportclient.TeleportClient
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
	log := r.Log.WithValues("cluster", req.NamespacedName)

	cluster := &capi.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, err
	}

	log.Info("Found cluster")

	// Skip if teleport secret already exists for the cluster
	secretName := "teleport-kube-agent-join-token"
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
		StringData: map[string]string{
			"joinToken": "join-token-here",
		},
	}

	secretNamespacedName := types.NamespacedName{
		Name:      secretName,
		Namespace: cluster.Namespace,
	}

	if err := r.Get(ctx, secretNamespacedName, secret); err != nil {
		// If the Secret does not exist
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Secret does not exist: %s", secretName))
			// Generate token from Teleport

			// Here you can add the code to create the Secret
			secret.StringData = map[string]string{
				"joinToken": "short-lived-join-token-goes-here",
			}
			if err := r.Create(ctx, secret); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to create Secret: %w", err)
			} else {
				log.Info(fmt.Sprintf("Secret created: %s", secretName))
			}
		} else {
			// If there was an error other than IsNotFound, return it
			return ctrl.Result{}, fmt.Errorf("failed to get Secret: %w", err)
		}
	} else {
		log.Info(fmt.Sprintf("Secret exists: %s. Cluster will be ignored", secretName))
	}

	// Here you can add the code to handle the case where the Secret exists
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// Uncomment the following line adding a pointer to an instance of the controlled resource as an argument
		For(&capi.Cluster{}).
		Complete(r)
}
