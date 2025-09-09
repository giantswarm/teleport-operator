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
	Client          client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	Teleport        *teleport.Teleport
	Namespace       string
	LastKnownConfig *config.Config
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

	log.Info("Processing teleport-operator ConfigMap change")

	// Parse the new configuration
	newConfig, err := config.GetConfigFromConfigMap(ctx, r.Client, r.Namespace)
	if err != nil {
		log.Error(err, "Failed to parse new configuration from ConfigMap")
		return ctrl.Result{}, microerror.Mask(err)
	}

	// Compare with last known configuration
	changes := r.detectConfigChanges(r.LastKnownConfig, newConfig)
	if len(changes) == 0 {
		log.V(1).Info("No meaningful configuration changes detected")
		return ctrl.Result{}, nil
	}

	log.Info("Configuration changes detected", "changes", changes)

	// Log all changes for audit purposes (before processing to ensure we don't lose them)
	for _, change := range changes {
		log.Info("Configuration change detected",
			"field", change.Field,
			"oldValue", change.OldValue,
			"newValue", change.NewValue,
			"impact", r.impactString(change.Impact))
	}

	// Update the Teleport instance configuration
	r.Teleport.Config = newConfig

	// Handle configuration changes based on their impact
	if err := r.handleConfigChanges(ctx, log, changes); err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	// Update cached configuration
	r.LastKnownConfig = newConfig

	log.Info("Successfully processed configuration changes")
	return ctrl.Result{}, nil
}

// ConfigChange represents a detected configuration change
type ConfigChange struct {
	Field    string
	OldValue string
	NewValue string
	Impact   ChangeImpact
}

// ChangeImpact defines the severity/impact of a configuration change
type ChangeImpact int

const (
	// ImpactLow affects only future operations
	ImpactLow ChangeImpact = iota
	// ImpactMedium requires ConfigMap updates
	ImpactMedium
	// ImpactHigh requires token invalidation
	ImpactHigh
	// ImpactCritical requires connection reset and token invalidation
	ImpactCritical
)

// detectConfigChanges compares old and new configurations and returns detected changes
func (r *ConfigReconciler) detectConfigChanges(oldConfig, newConfig *config.Config) []ConfigChange {
	var changes []ConfigChange

	if oldConfig == nil {
		// First time seeing config during startup - we don't treat this as "changes"
		// since the system is initializing and no operational changes are needed.
		// All components will use the new config through normal startup flow.
		return changes
	}

	// Check ProxyAddr - critical change requiring reconnection
	if oldConfig.ProxyAddr != newConfig.ProxyAddr {
		changes = append(changes, ConfigChange{
			Field:    "ProxyAddr",
			OldValue: oldConfig.ProxyAddr,
			NewValue: newConfig.ProxyAddr,
			Impact:   ImpactCritical,
		})
	}

	// Check ManagementClusterName - high impact requiring token regeneration
	if oldConfig.ManagementClusterName != newConfig.ManagementClusterName {
		changes = append(changes, ConfigChange{
			Field:    "ManagementClusterName",
			OldValue: oldConfig.ManagementClusterName,
			NewValue: newConfig.ManagementClusterName,
			Impact:   ImpactHigh,
		})
	}

	// Check TeleportVersion - medium impact requiring ConfigMap updates
	if oldConfig.TeleportVersion != newConfig.TeleportVersion {
		changes = append(changes, ConfigChange{
			Field:    "TeleportVersion",
			OldValue: oldConfig.TeleportVersion,
			NewValue: newConfig.TeleportVersion,
			Impact:   ImpactMedium,
		})
	}

	// Check AppName - medium impact affecting ConfigMap names
	if oldConfig.AppName != newConfig.AppName {
		changes = append(changes, ConfigChange{
			Field:    "AppName",
			OldValue: oldConfig.AppName,
			NewValue: newConfig.AppName,
			Impact:   ImpactMedium,
		})
	}

	// Check AppVersion - low impact, only affects new deployments
	if oldConfig.AppVersion != newConfig.AppVersion {
		changes = append(changes, ConfigChange{
			Field:    "AppVersion",
			OldValue: oldConfig.AppVersion,
			NewValue: newConfig.AppVersion,
			Impact:   ImpactLow,
		})
	}

	// Check AppCatalog - low impact, only affects new deployments
	if oldConfig.AppCatalog != newConfig.AppCatalog {
		changes = append(changes, ConfigChange{
			Field:    "AppCatalog",
			OldValue: oldConfig.AppCatalog,
			NewValue: newConfig.AppCatalog,
			Impact:   ImpactLow,
		})
	}

	return changes
}

// handleConfigChanges processes detected configuration changes
func (r *ConfigReconciler) handleConfigChanges(ctx context.Context, log logr.Logger, changes []ConfigChange) error {
	// Determine the highest impact change
	maxImpact := ImpactLow
	for _, change := range changes {
		if change.Impact > maxImpact {
			maxImpact = change.Impact
		}
	}

	// Handle changes based on impact level

	switch {
	case maxImpact >= ImpactCritical:
		log.Info("Critical configuration change detected, forcing reconnection and token invalidation")
		// Critical changes require clearing cached identity to force reconnection
		r.Teleport.Identity = nil
		return r.triggerClusterReconciliation(ctx, log, "Critical config change - identity cleared, tokens will be regenerated")

	case maxImpact >= ImpactHigh:
		log.Info("High impact configuration change detected, invalidating tokens")
		return r.triggerClusterReconciliation(ctx, log, "High impact config change - tokens will be regenerated")

	case maxImpact >= ImpactMedium:
		log.Info("Medium impact configuration change detected, updating ConfigMaps")
		return r.triggerClusterReconciliation(ctx, log, "Medium impact config change - ConfigMaps will be updated")

	case maxImpact >= ImpactLow:
		log.Info("Low impact configuration change detected, no immediate action required")
		// Low impact changes (AppVersion, AppCatalog) only affect future deployments
		return nil

	default:
		return nil
	}
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

// impactString returns a human-readable string for the impact level
func (r *ConfigReconciler) impactString(impact ChangeImpact) string {
	switch impact {
	case ImpactLow:
		return "low"
	case ImpactMedium:
		return "medium"
	case ImpactHigh:
		return "high"
	case ImpactCritical:
		return "critical"
	default:
		return "unknown"
	}
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
