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
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"
)

// ConfigReconciler reconciles changes to the teleport-operator ConfigMap
type ConfigReconciler struct {
	Client    client.Client
	Log       logr.Logger
	Scheme    *runtime.Scheme
	Teleport  *teleport.Teleport
	Namespace string
}

//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=configmaps/status,verbs=get

// Reconcile handles ConfigMap changes for the teleport-operator configuration
func (r *ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("configmap", req.NamespacedName)

	configMap := &corev1.ConfigMap{}
	if err := r.Client.Get(ctx, req.NamespacedName, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("ConfigMap deleted, operator will continue with existing configuration")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, microerror.Mask(err)
	}

	log.Info("ConfigMap change detected, triggering cluster reconciliation")

	// Parse the new configuration
	newConfig, err := config.GetConfigFromConfigMap(ctx, r.Client, r.Namespace)
	if err != nil {
		log.Error(err, "Failed to parse new configuration from ConfigMap")
		return ctrl.Result{}, microerror.Mask(err)
	}

	// Update the Teleport instance configuration
	r.Teleport.Config = newConfig

	// Trigger immediate reconciliation of all clusters
	if err := r.triggerClusterReconciliation(ctx, log, "ConfigMap updated"); err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	log.Info("Successfully processed ConfigMap change")
	return ctrl.Result{}, nil
}

// triggerClusterReconciliation forces immediate reconciliation of all cluster resources
func (r *ConfigReconciler) triggerClusterReconciliation(ctx context.Context, log logr.Logger, reason string) error {
	// List all clusters
	clusterList := &capi.ClusterList{}
	if err := r.Client.List(ctx, clusterList); err != nil {
		return microerror.Mask(err)
	}

	log.Info("Triggering immediate reconciliation for all clusters",
		"clusterCount", len(clusterList.Items),
		"reason", reason)

	// Force reconciliation by adding/updating an annotation
	timestamp := time.Now().Format(time.RFC3339)

	for i := range clusterList.Items {
		cluster := &clusterList.Items[i]

		if cluster.Annotations == nil {
			cluster.Annotations = make(map[string]string)
		}

		// Add annotation to trigger reconciliation
		cluster.Annotations[key.ConfigUpdateAnnotation] = timestamp

		if err := r.Client.Update(ctx, cluster); err != nil {
			log.Error(err, "Failed to update cluster to trigger reconciliation",
				"cluster", cluster.Name, "namespace", cluster.Namespace)
			// Continue with other clusters even if one fails
			continue
		}

		log.V(1).Info("Triggered reconciliation for cluster",
			"cluster", cluster.Name, "namespace", cluster.Namespace)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a predicate to only watch the specific ConfigMap we care about
	configMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		if configMap, ok := object.(*corev1.ConfigMap); ok {
			return configMap.Name == key.TeleportOperatorConfigName &&
				configMap.Namespace == r.Namespace
		}
		return false
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithEventFilter(configMapPredicate).
		Complete(r)
}
