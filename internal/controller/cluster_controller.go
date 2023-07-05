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

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/teleportapp"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleportclient"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	client.Client
	Log            logr.Logger
	Scheme         *runtime.Scheme
	TeleportClient *teleportclient.TeleportClient
	TeleportApp    *teleportapp.TeleportApp
}

const finalizerName string = "teleport.finalizer.giantswarm.io"

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

	if !cluster.DeletionTimestamp.IsZero() {
		if containsString(cluster.GetFinalizers(), finalizerName) {
			clusterName := cluster.Name
			if clusterName != r.TeleportClient.ManagementClusterName {
				clusterName = fmt.Sprintf("%s-%s", r.TeleportClient.ManagementClusterName, clusterName)
			}

			err := r.ensureClusterDeregistered(ctx, log, clusterName)
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}

			// Remove the finalizer
			controllerutil.RemoveFinalizer(cluster, finalizerName)
			// Remember to update the cluster
			if err := r.Update(context.Background(), cluster); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		}
		return ctrl.Result{}, nil
	}

	if _, err := r.TeleportClient.GetClient(ctx); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get Teleport client: %w", err)
	}
	log.Info("Teleport client connected")

	secretName := fmt.Sprintf("%s-teleport-join-token", cluster.Name) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
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
			joinToken, err := r.TeleportClient.GetToken(ctx, cluster.Name)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to generate join token: %w", err)
			}
			log.Info("Join token generated")
			secret.StringData = map[string]string{
				"joinToken": joinToken,
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
		log.Info(fmt.Sprintf("Secret exists: %s", secretName))
		// Update secret if token expired or is expiring
		hasExpired, err := r.TeleportClient.HasTokenExpired(ctx, cluster.Name)
		if err != nil {
			return ctrl.Result{}, fmt.Errorf("failed to verify token expiry: %w", err)
		}
		if hasExpired {
			log.Info("Join token expired")
			joinToken, err := r.TeleportClient.GetToken(ctx, cluster.Name)
			if err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to generate teleport token: %w", err)
			}
			log.Info("Join token generated")
			secret.StringData = map[string]string{
				"joinToken": joinToken,
			}
			if err := r.Update(ctx, secret); err != nil {
				return ctrl.Result{}, fmt.Errorf("failed to update Secret: %w", err)
			} else {
				log.Info(fmt.Sprintf("Secret updated: %s", secretName))
			}
		} else {
			log.Info("Join token is valid, nothing to do.")
		}
	}

	// Register teleport for MC/WC cluster
	clusterName := cluster.Name
	namespace := "giantswarm"

	mc := true
	if clusterName != r.TeleportClient.ManagementClusterName {
		namespace = cluster.Namespace
		mc = false
	}

	if err := r.ensureClusterRegistered(ctx, log, clusterName, r.TeleportClient.ManagementClusterName, namespace, mc); err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	if !containsString(cluster.GetFinalizers(), finalizerName) {
		controllerutil.AddFinalizer(cluster, finalizerName)
		if err := r.Update(context.Background(), cluster); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	}

	// Here you can add the code to handle the case where the Secret exists
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}

func (r *ClusterReconciler) ensureClusterRegistered(ctx context.Context, log logr.Logger, clusterName string, managementClusterName string, namespace string, mc bool) error {
	log.Info("Checking if cluster is already registered in teleport")

	_clusterName := clusterName
	if !mc {
		_clusterName = fmt.Sprintf("%s-%s", r.TeleportClient.ManagementClusterName, clusterName)
	}
	exists, _, err := r.TeleportClient.ClusterExists(ctx, _clusterName)
	if err != nil {
		return microerror.Mask(err)
	}

	if !exists {
		log.Info("Cluster does not exists in teleport")

		joinToken, err := r.TeleportClient.GetToken(ctx, _clusterName)
		if err != nil {
			return fmt.Errorf("Failed to generate join token: %w", err)
		}

		log.Info("Installing teleport kube agent app in cluster")

		err = r.TeleportApp.EnsureApp(ctx, namespace, clusterName, managementClusterName, joinToken, mc)
		if err != nil {
			return microerror.Mask(err)
		}

		log.Info("Installed teleport kube agent app in cluster")
		log.Info("Cluster registered in teleport")
	} else {
		log.Info("Cluster exists in teleport")
	}

	return nil
}

func (r *ClusterReconciler) ensureClusterDeregistered(ctx context.Context, log logr.Logger, clusterName string) error {
	log.Info("Checking if cluster is registered in teleport")

	exists, ks, err := r.TeleportClient.ClusterExists(ctx, clusterName)
	if err != nil {
		return microerror.Mask(err)
	}

	if !exists {
		log.Info("Cluster does not exists in teleport")
		return nil
	}

	log.Info("De-registering teleport kube agent app in cluster")

	if err := r.TeleportClient.DeregisterCluster(ctx, ks); err != nil {
		return microerror.Mask(err)
	}
	log.Info("Cluster de-registered from teleport")

	return nil
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}
