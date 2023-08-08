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

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
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
	log := r.Log.WithValues("cluster", req.NamespacedName)

	cluster := &capi.Cluster{}
	if err := r.Client.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, microerror.Mask(err)
	}

	log.Info("Reconciling cluster", "clusterName", cluster.GetName())

	var (
		installNamespace    string
		registerName        string
		isManagementCluster bool
	)

	if cluster.Name == r.Teleport.SecretConfig.ManagementClusterName {
		isManagementCluster = true
		installNamespace = key.MCTeleportAppDefaultNamespace
		registerName = cluster.Name
	} else {
		isManagementCluster = false
		installNamespace = cluster.Namespace
		registerName = key.GetRegisterName(r.Teleport.SecretConfig.ManagementClusterName, cluster.Name)
	}

	teleportConfig := &teleport.TeleportConfig{
		Log:                 log,
		CtrlClient:          r.Client,
		Cluster:             cluster,
		RegisterName:        registerName,
		InstallNamespace:    installNamespace,
		IsManagementCluster: isManagementCluster,
	}

	// Check if the cluster instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	// if it is, delete the cluster from teleport
	if !cluster.DeletionTimestamp.IsZero() {
		// Remove finalizer from the Cluster CR
		if controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
			if err := teleport.RemoveFinalizer(ctx, teleportConfig); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		}

		// Delete teleport token for the cluster
		if err := r.Teleport.DeleteToken(ctx, teleportConfig); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		// Delete teleport kubernetes resource for the cluster
		if err := r.Teleport.DeleteClusterFromTeleport(ctx, teleportConfig); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		// Delete Secret for the cluster
		if err := r.Teleport.DeleteSecret(ctx, teleportConfig); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		// Delete ConfigMap for the cluster
		if err := r.Teleport.DeleteConfigMap(ctx, teleportConfig); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		return ctrl.Result{}, nil
	}

	// Add finalizer to cluster CR if it's not there
	if !controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
		if err := teleport.AddFinalizer(ctx, teleportConfig); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	}

	// Check if the secret exists in the cluster, if not, generate teleport token and create the secret
	// if it is, check teleport token validity, and update the secret if teleport token has expired
	secret, err := r.Teleport.GetSecret(ctx, teleportConfig)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}
	if secret == nil {
		token, err := r.Teleport.GenerateToken(ctx, teleportConfig, "node")
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if err := r.Teleport.CreateSecret(ctx, teleportConfig, token); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	} else {
		token, err := r.Teleport.GetTokenFromSecret(ctx, secret)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		tokenValid, err := r.Teleport.IsTokenValid(ctx, teleportConfig, token, "node")
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if !tokenValid {
			token, err := r.Teleport.GenerateToken(ctx, teleportConfig, "node")
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			if err := r.Teleport.UpdateSecret(ctx, teleportConfig, token); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		} else {
			log.Info("Secret has valid teleport join token", "secretName", secret.GetName())
		}
	}

	// Check if the cluster is registered in teleport, if not, check if app is teleport-kube-agent app installed
	// if app is not installed, installed it
	clusterRegisteredInTeleport, err := r.Teleport.IsClusterRegisteredInTeleport(ctx, teleportConfig)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	if !clusterRegisteredInTeleport {
		// Check if teleport-kube-agent app is installed for the cluster, if not,
		// create configmap with newly generated teleport token and install the app
		kubeAgentAppInstalled, err := r.Teleport.IsKubeAgentAppInstalled(ctx, teleportConfig)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if !kubeAgentAppInstalled {
			token, err := r.Teleport.GenerateToken(ctx, teleportConfig, "kube")
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			if err := r.Teleport.CreateConfigMap(ctx, teleportConfig, token); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			if err := r.Teleport.InstallKubeAgentApp(ctx, teleportConfig); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		}
	}

	// We need to requeue to check the teleport token validity
	// and update secret for the cluster, if it expires
	return ctrl.Result{RequeueAfter: 1 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}
