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
	"time"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util/patch"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
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
	TeleportApp    *teleportapp.TeleportApp
	TeleportClient *teleportclient.TeleportClient
}

type ClusterRegisterConfig struct {
	ClusterName         string
	RegisterName        string
	InstallNamespace    string
	IsManagementCluster bool
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

	// Check if the cluster instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	// if it is, delete the cluster from teleport
	if !cluster.DeletionTimestamp.IsZero() {
		return ctrl.Result{}, r.ensureClusterDeletion(ctx, log, cluster)
	}

	// Register teleport for MC/WC
	if err := r.registerTeleport(ctx, log, cluster); err != nil {
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

func (r *ClusterReconciler) ensureClusterDeletion(ctx context.Context, log logr.Logger, cluster *capi.Cluster) error {
	if controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
		if err := r.deleteSecret(ctx, log, cluster); err != nil {
			return microerror.Mask(err)
		}
		if err := r.deleteConfigMap(ctx, log, cluster); err != nil {
			return microerror.Mask(err)
		}

		registerName := key.GetRegisterName(r.TeleportClient.ManagementClusterName, cluster.Name)
		if cluster.Name == r.TeleportClient.ManagementClusterName {
			registerName = cluster.Name
		}

		if err := r.ensureClusterDeregistered(ctx, log, registerName); err != nil {
			return microerror.Mask(err)
		}
		patchHelper, err := patch.NewHelper(cluster, r.Client)
		if err != nil {
			return errors.WithStack(err)
		}
		controllerutil.RemoveFinalizer(cluster, key.TeleportOperatorFinalizer)
		if err := patchHelper.Patch(ctx, cluster); err != nil {
			log.Error(err, "failed to remove finalizer.")
			return microerror.Mask(client.IgnoreNotFound(err))
		}
		log.Info("Successfully removed finalizer.", "finalizer_name", key.TeleportOperatorFinalizer)
	}
	return nil
}

func (r *ClusterReconciler) ensureClusterDeregistered(ctx context.Context, log logr.Logger, registerName string) error {
	log.Info("Checking if cluster is registered in teleport...")
	exists, ks, err := r.TeleportClient.IsClusterRegistered(ctx, registerName)
	if err != nil {
		return microerror.Mask(err)
	}

	if !exists {
		log.Info("Cluster does not exist in teleport.")
		return nil
	}

	log.Info("De-registering cluster from teleport...")
	if err := r.TeleportClient.DeregisterCluster(ctx, ks); err != nil {
		return microerror.Mask(err)
	}
	log.Info("Cluster de-registered from teleport.")

	return nil
}

func (r *ClusterReconciler) ensureSecret(ctx context.Context, log logr.Logger, config *ClusterRegisterConfig) error {
	secretName := key.GetSecretName(config.ClusterName) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: config.InstallNamespace,
		},
	}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: config.InstallNamespace}, secret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info(fmt.Sprintf("Secret does not exist: %s", secretName))
			joinToken, err := r.generateJoinToken(ctx, config.RegisterName)
			if err != nil {
				return microerror.Mask(err)
			}
			log.Info("Generated node join token.")
			secret.StringData = map[string]string{
				"joinToken": joinToken,
			}
			if err := r.Create(ctx, secret); err != nil {
				return microerror.Mask(fmt.Errorf("failed to create Secret: %w", err))
			} else {
				log.Info(fmt.Sprintf("Secret created: %s", secretName))
				return nil
			}
		} else {
			return microerror.Mask(fmt.Errorf("failed to get Secret: %w", err))
		}
	}

	log.Info(fmt.Sprintf("Secret exists: %s", secretName))

	oldTokenBytes, ok := secret.Data["joinToken"]
	if !ok {
		log.Info("failed to get joinToken from Secret: %s", secretName)
	}

	isTokenValid, err := r.TeleportClient.IsTokenValid(ctx, string(oldTokenBytes), config.RegisterName)
	if err != nil {
		return microerror.Mask(fmt.Errorf("failed to verify token validity: %w", err))
	}
	if !isTokenValid {
		log.Info("Join token has expired.")
		joinToken, err := r.generateJoinToken(ctx, config.RegisterName)
		if err != nil {
			return microerror.Mask(err)
		}
		log.Info("Join token re-generated")
		secret.StringData = map[string]string{
			"joinToken": joinToken,
		}
		if err := r.Update(ctx, secret); err != nil {
			return microerror.Mask(fmt.Errorf("failed to update Secret: %w", err))
		} else {
			log.Info(fmt.Sprintf("Secret updated: %s", secretName))
		}
	} else {
		log.Info("Join token is valid, nothing to do.")
	}
	return nil
}

func (r *ClusterReconciler) deleteSecret(ctx context.Context, log logr.Logger, cluster *capi.Cluster) error {
	secretName := key.GetSecretName(cluster.Name) //#nosec G101
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretName,
			Namespace: cluster.Namespace,
		},
	}

	log.Info("Deleting secret...")
	if err := r.Delete(ctx, secret); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("Secret does not exists.")
			return nil
		}

		return microerror.Mask(fmt.Errorf("failed to create Secret: %w", err))
	}

	log.Info("Secret deleted.")
	return nil
}

func (r *ClusterReconciler) registerTeleport(ctx context.Context, log logr.Logger, cluster *capi.Cluster) error {
	var installNamespace string
	var registerName string
	var isManagementCluster bool

	if cluster.Name == r.TeleportClient.ManagementClusterName {
		isManagementCluster = true
		installNamespace = key.MCTeleportAppDefaultNamespace
		registerName = cluster.Name
	} else {
		isManagementCluster = false
		installNamespace = cluster.Namespace
		registerName = key.GetRegisterName(r.TeleportClient.ManagementClusterName, cluster.Name)
	}

	clusterRegisterConfig := ClusterRegisterConfig{
		ClusterName:         cluster.Name,
		RegisterName:        registerName,
		InstallNamespace:    installNamespace,
		IsManagementCluster: isManagementCluster,
	}

	if err := r.ensureSecret(ctx, log, &clusterRegisterConfig); err != nil {
		return microerror.Mask(err)
	}

	if err := r.ensureClusterRegistered(ctx, log, &clusterRegisterConfig); err != nil {
		return microerror.Mask(err)
	}

	if !controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
		patchHelper, err := patch.NewHelper(cluster, r.Client)
		if err != nil {
			return errors.WithStack(err)
		}
		controllerutil.AddFinalizer(cluster, key.TeleportOperatorFinalizer)
		if err := patchHelper.Patch(ctx, cluster); err != nil {
			log.Error(err, "failed to add finalizer.")
			return microerror.Mask(client.IgnoreNotFound(err))
		}
		log.Info("Successfully added finalizer.", "finalizer_name", key.TeleportOperatorFinalizer)
	}
	return nil
}

func (r *ClusterReconciler) ensureClusterRegistered(ctx context.Context, log logr.Logger, config *ClusterRegisterConfig) error {
	isRegistered, _, err := r.TeleportClient.IsClusterRegistered(ctx, config.RegisterName)
	if err != nil {
		return microerror.Mask(err)
	}

	if isRegistered {
		log.Info("Cluster is registered in teleport.")
		return nil
	}
	log.Info("Registering cluster in teleport...")

	joinToken, err := r.generateJoinToken(ctx, config.RegisterName)
	if err != nil {
		return microerror.Mask(err)
	}

	installAppConfig := teleportapp.AppConfig{
		InstallNamespace:    config.InstallNamespace,
		RegisterName:        config.RegisterName,
		ClusterName:         config.ClusterName,
		JoinToken:           joinToken,
		IsManagementCluster: config.IsManagementCluster,
	}
	err = r.TeleportApp.InstallApp(ctx, &installAppConfig)
	if err != nil {
		return microerror.Mask(err)
	}

	return nil
}

func (r *ClusterReconciler) generateJoinToken(ctx context.Context, registerName string) (string, error) {
	joinToken, err := r.TeleportClient.GetToken(ctx, registerName)
	if err != nil {
		return "", microerror.Mask(fmt.Errorf("failed to generate token: %w", err))
	}
	return joinToken, nil
}

func (r *ClusterReconciler) deleteConfigMap(ctx context.Context, log logr.Logger, cluster *capi.Cluster) error {
	log.Info("Deleting config map...")
	configMapName := key.GetConfigmapName(cluster.Name, r.TeleportClient.AppName)

	cm := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: cluster.Namespace,
		},
	}
	if err := r.Delete(ctx, &cm); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ConfigMap does not exist.")
			return nil
		}

		return microerror.Mask(err)
	}
	log.Info("ConfigMap deleted.")
	return nil
}
