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

	registerName := cluster.Name
	if cluster.Name != r.Teleport.SecretConfig.ManagementClusterName {
		registerName = key.GetRegisterName(r.Teleport.SecretConfig.ManagementClusterName, cluster.Name)
	}

	// Check if the cluster instance is marked to be deleted, which is indicated by the deletion timestamp being set.
	// if it is, delete the cluster from teleport
	if !cluster.DeletionTimestamp.IsZero() {
		// Delete teleport token for the cluster
		if err := r.Teleport.DeleteToken(ctx, log, registerName); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		// Delete Secret for the cluster
		if err := r.Teleport.DeleteSecret(ctx, log, r.Client, cluster.Name, cluster.Namespace); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		// Delete ConfigMap for the cluster
		if err := r.Teleport.DeleteConfigMap(ctx, log, r.Client, cluster.Name, cluster.Namespace); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		// Remove finalizer from the Cluster CR
		if controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
			if err := teleport.RemoveFinalizer(ctx, log, cluster, r.Client); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		}

		return ctrl.Result{}, nil
	}

	// Add finalizer to cluster CR if it's not there
	if !controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
		if err := teleport.AddFinalizer(ctx, log, cluster, r.Client); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	}

	// Check if the secret exists in the cluster, if not, generate teleport token and create the secret
	// if it is, check teleport token validity, and update the secret if teleport token has expired
	secret, err := r.Teleport.GetSecret(ctx, log, r.Client, cluster.Name, cluster.Namespace)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}
	if secret == nil {
		token, err := r.Teleport.GenerateToken(ctx, registerName, "node")
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if err := r.Teleport.CreateSecret(ctx, log, r.Client, cluster.Name, cluster.Namespace, token); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
	} else {
		token, err := r.Teleport.GetTokenFromSecret(ctx, secret)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		tokenValid, err := r.Teleport.IsTokenValid(ctx, registerName, token, "node")
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if !tokenValid {
			token, err := r.Teleport.GenerateToken(ctx, registerName, "node")
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			if err := r.Teleport.UpdateSecret(ctx, log, r.Client, cluster.Name, cluster.Namespace, token); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		} else {
			log.Info("Secret has valid teleport node join token", "secretName", secret.GetName())
		}
	}

	// Check if the confimap exists in the cluster, if not, generate teleport token and create the secret
	// if it is, check teleport token validity, and update the configmap if teleport token has expired
	configMap, err := r.Teleport.GetConfigMap(ctx, log, r.Client, cluster.Name, cluster.Namespace)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	if configMap != nil {
		token, err := r.Teleport.GetTokenFromConfigMap(ctx, configMap)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		tokenValid, err := r.Teleport.IsTokenValid(ctx, registerName, token, "kube")
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if !tokenValid {
			token, err := r.Teleport.GenerateToken(ctx, registerName, "kube")
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			if err := r.Teleport.UpdateConfigMap(ctx, log, r.Client, configMap, token); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		} else {
			log.Info("ConfigMap has valid teleport kube join token", "configMapName", configMap.GetName())
		}
	} else {
		token, err := r.Teleport.GenerateToken(ctx, registerName, "kube")
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if err := r.Teleport.CreateConfigMap(ctx, log, r.Client, cluster.Name, cluster.Namespace, registerName, token); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
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
