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
	capi "sigs.k8s.io/cluster-api/api/core/v1beta2"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/giantswarm/teleport-operator/internal/pkg/config"
	"github.com/giantswarm/teleport-operator/internal/pkg/key"
	"github.com/giantswarm/teleport-operator/internal/pkg/teleport"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const identityExpirationPeriod = 20 * time.Minute

// ClusterReconciler reconciles a Cluster object
type ClusterReconciler struct {
	Client client.Client
	// APIReader reads straight from the API server, bypassing the manager's
	// cache. Read-modify-write on an object several reconcilers share must not
	// re-read from an eventually consistent cache: after a conflict the cache
	// can still serve the copy we just lost against, so the retry would merge
	// stale data and drop the winner's entry.
	APIReader         client.Reader
	Log               logr.Logger
	Scheme            *runtime.Scheme
	Teleport          *teleport.Teleport
	IsBotEnabled      bool
	Namespace         string
	lastAssignedRoles []string
}

// desiredTbotOutputs projects the cluster list into the tbot outputs map. The
// aggregate ConfigMap is derived state, so it is rebuilt from the authoritative
// list rather than mutated per cluster - which is what makes an orphaned entry
// (a cluster deleted while the operator was down, say) heal by itself.
func (r *ClusterReconciler) desiredTbotOutputs(ctx context.Context) (map[string]string, error) {
	clusters := &capi.ClusterList{}
	if err := r.Client.List(ctx, clusters); err != nil {
		return nil, microerror.Mask(err)
	}
	outputs := make(map[string]string, len(clusters.Items))
	for i := range clusters.Items {
		cluster := &clusters.Items[i]
		if !cluster.DeletionTimestamp.IsZero() {
			continue
		}
		outputs[key.RegisterName(r.Teleport.Config.ManagementClusterName, cluster.Name)] = cluster.Name
	}
	return outputs, nil
}

// syncTbotOutputs refreshes the aggregate ConfigMap. Nothing consumes it yet, so
// a failure must not fail the reconcile - every other side effect is already
// persisted by this point, and the next pass rebuilds the map from scratch.
func (r *ClusterReconciler) syncTbotOutputs(ctx context.Context, log logr.Logger) {
	if !r.IsBotEnabled {
		return
	}
	outputs, err := r.desiredTbotOutputs(ctx)
	if err != nil {
		log.Error(err, "tbot: could not list clusters for the aggregate outputs configmap")
		return
	}
	if err := r.Teleport.SetTbotOutputs(ctx, log, r.Client, outputs); err != nil {
		log.Error(err, "tbot: could not write the aggregate outputs configmap")
	}
}

//+kubebuilder:rbac:groups=cluster.x-k8s.io.giantswarm.io,resources=clusters,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=cluster.x-k8s.io.giantswarm.io,resources=clusters/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=cluster.x-k8s.io.giantswarm.io,resources=clusters/finalizers,verbs=update
//+kubebuilder:rbac:groups=helm.toolkit.fluxcd.io,resources=helmreleases,verbs=get;list;watch;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
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

	log.Info("Reconciling cluster")

	appsEnabled, err := r.Teleport.AreTeleportAppsEnabled(ctx, cluster.Name, cluster.Namespace)
	if err != nil {
		log.Error(err, "Failed to check if Teleport apps are enabled")
		return ctrl.Result{}, microerror.Mask(err)
	}

	roles := []string{key.RoleKube}
	if appsEnabled {
		roles = append(roles, key.RoleApp)
	}
	r.lastAssignedRoles = roles
	if r.Teleport.Identity != nil {
		log.Info("Teleport identity", "last-read-minutes-ago", r.Teleport.Identity.Age(), "hash", r.Teleport.Identity.Hash())
	}

	if r.Teleport.Identity == nil || time.Since(r.Teleport.Identity.LastRead) > identityExpirationPeriod {
		log.Info("Retrieving new identity", "secretName", key.TeleportBotSecretName)

		newIdentityConfig, err := config.GetIdentityConfigFromSecret(ctx, r.Client, r.Namespace)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		if r.Teleport.TeleportClient, err = teleport.NewClient(ctx, r.Teleport.Config.ProxyAddr, newIdentityConfig.IdentityFile); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if r.Teleport.Identity == nil {
			log.Info("Connected to teleport cluster", "proxyAddr", r.Teleport.Config.ProxyAddr)
		} else {
			log.Info("Re-connected to teleport cluster with new identity", "proxyAddr", r.Teleport.Config.ProxyAddr)
		}
		r.Teleport.Identity = newIdentityConfig
	}

	registerName := key.RegisterName(r.Teleport.Config.ManagementClusterName, cluster.Name)

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

		if r.IsBotEnabled {
			botMgr, err := teleport.NewTeleportAppConfigManager(ctx, r.Client,
				key.TeleportBotAppName,
				key.TeleportBotNamespace,
				key.GetTbotConfigmapName(cluster.Name))
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			if err := botMgr.DeleteConfig(ctx, log); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}

			if err := r.Teleport.DeleteTbotConfigMap(ctx, log, r.Client, cluster.Name, key.TeleportBotNamespace); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}

			if err := r.Teleport.DeleteKubeconfigSecret(ctx, log, r.Client, cluster.Name, key.TeleportBotNamespace); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}

			// The deleting cluster is already excluded from the projection.
			r.syncTbotOutputs(ctx, log)
		}

		kubeAgentMgr, err := teleport.NewTeleportAppConfigManager(ctx, r.Client,
			key.GetAppName(cluster.Name, r.Teleport.Config.AppName),
			cluster.Namespace,
			key.GetConfigmapName(cluster.Name, r.Teleport.Config.AppName))
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if err := kubeAgentMgr.DeleteConfig(ctx, log); err != nil {
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

	// Check and update Secret if necessary
	secret, err := r.Teleport.GetSecret(ctx, log, r.Client, cluster.Name, cluster.Namespace)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}
	if secret == nil {
		token, err := r.Teleport.GenerateToken(ctx, registerName, []string{key.RoleNode})
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
		tokenValid, err := r.Teleport.IsTokenValid(ctx, registerName, token, key.RoleNode)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if !tokenValid {
			token, err := r.Teleport.GenerateToken(ctx, registerName, []string{key.RoleNode})
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

	// Look up the deployed teleport-kube-agent chart version for this cluster.
	// The layout of the values ConfigMap we write depends on it: nested-only
	// for v0.11.0+, dual (flat + nested) for older or unknown versions.
	tkaResourceName := key.GetAppName(cluster.Name, r.Teleport.Config.AppName)
	tkaVersion, err := teleport.GetTeleportKubeAgentVersion(ctx, r.Client, tkaResourceName, cluster.Namespace)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}
	log = log.WithValues("tkaVersion", tkaVersion)

	// Check if the configmap exists in the cluster, if not, generate teleport token and create the config map
	// if it is, check teleport token validity, and update the configmap if teleport token has expired
	configMap, err := r.Teleport.GetConfigMap(ctx, log, r.Client, cluster.Name, cluster.Namespace)
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	if configMap == nil {
		token, err := r.Teleport.GenerateToken(ctx, registerName, roles)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if err := r.Teleport.CreateConfigMap(ctx, log, r.Client, cluster.Name, cluster.Namespace, registerName, token, roles, tkaVersion); err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		log.Info("Created new config map with teleport join token", "configMapName", key.GetConfigmapName(cluster.Name, r.Teleport.Config.AppName), "roles", roles)
	} else {
		token, err := r.Teleport.GetTokenFromConfigMap(ctx, configMap)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		tokenValid, err := r.Teleport.IsTokenValid(ctx, registerName, token, key.RolesToString(roles))
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}

		writeToken := token
		if !tokenValid {
			writeToken, err = r.Teleport.GenerateToken(ctx, registerName, roles)
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		}

		// Single drift check: compare the stored values document to what the
		// template would produce now. This catches token rotation, teleport
		// version drift, and layout changes (dual ↔ nested-only) in one shot.
		desiredValues := r.Teleport.RenderConfigMapValues(registerName, writeToken, roles, tkaVersion)

		switch {
		case configMap.Data["values"] == desiredValues:
			log.Info("ConfigMap has valid teleport join token", "configMapName", configMap.GetName(), "roles", roles)
		case !tokenValid:
			if err := r.Teleport.UpdateConfigMap(ctx, log, r.Client, configMap, writeToken, roles, tkaVersion); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			log.Info("Updated config map with new teleport join token", "configMapName", configMap.GetName(), "roles", roles)
		default:
			if err := r.Teleport.UpdateConfigMap(ctx, log, r.Client, configMap, writeToken, roles, tkaVersion); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			log.Info("Updated config map to align teleport version and values layout",
				"configMapName", configMap.GetName(),
				"teleportVersion", r.Teleport.Config.TeleportVersion,
				"nestedValuesOnly", key.UsesNestedKubeAgentValues(tkaVersion))
		}
	}

	kubeAgentMgr, err := teleport.NewTeleportAppConfigManager(ctx, r.Client,
		key.GetAppName(cluster.Name, r.Teleport.Config.AppName),
		cluster.Namespace,
		key.GetConfigmapName(cluster.Name, r.Teleport.Config.AppName))
	if err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}
	if err := kubeAgentMgr.EnsureConfig(ctx, log); err != nil {
		return ctrl.Result{}, microerror.Mask(err)
	}

	if r.IsBotEnabled {
		secret, err := r.Teleport.GetKubeconfigSecret(ctx, r.Client, cluster.Name, key.TeleportBotNamespace)
		if err != nil {
			return ctrl.Result{}, microerror.Mask(err)
		}
		if secret == nil {
			if err := r.Teleport.EnsureTbotConfigMap(ctx, log, r.Client, cluster.Name, key.TeleportBotNamespace, registerName); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}

			botMgr, err := teleport.NewTeleportAppConfigManager(ctx, r.Client,
				key.TeleportBotAppName,
				key.TeleportBotNamespace,
				key.GetTbotConfigmapName(cluster.Name))
			if err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
			if err := botMgr.EnsureConfig(ctx, log); err != nil {
				return ctrl.Result{}, microerror.Mask(err)
			}
		}

		// Maintained alongside the per-cluster ConfigMaps until the consumer is
		// switched over. Outside the `secret == nil` gate: the entry must exist
		// for as long as the cluster does.
		r.syncTbotOutputs(ctx, log)
	}

	// We need to requeue to check the teleport token validity
	// and update secret for the cluster, if it expires
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&capi.Cluster{}).
		Complete(r)
}
