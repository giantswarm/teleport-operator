package teleport

import (
	"context"

	"github.com/go-logr/logr"
	tc "github.com/gravitational/teleport/api/client"
	tt "github.com/gravitational/teleport/api/types"
	"github.com/pkg/errors"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/giantswarm/microerror"
	capi "sigs.k8s.io/cluster-api/api/v1beta1"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

type Teleport struct {
	Config     *Config
	Logger     logr.Logger
	CtrlClient client.Client
	Client     *tc.Client
	Namespace  string
}

type ClusterRegisterConfig struct {
	ClusterName         string
	RegisterName        string
	InstallNamespace    string
	IsManagementCluster bool
}

func New(namespace string) *Teleport {
	return &Teleport{
		Namespace: namespace,
	}
}

func (t *Teleport) EnsureClusterRegistered(ctx context.Context, cluster *capi.Cluster) error {
	var installNamespace string
	var registerName string
	var isManagementCluster bool

	if cluster.Name == t.Config.ManagementClusterName {
		isManagementCluster = true
		installNamespace = key.MCTeleportAppDefaultNamespace
		registerName = cluster.Name
	} else {
		isManagementCluster = false
		installNamespace = cluster.Namespace
		registerName = key.GetRegisterName(t.Config.ManagementClusterName, cluster.Name)
	}

	clusterRegisterConfig := ClusterRegisterConfig{
		ClusterName:         cluster.Name,
		RegisterName:        registerName,
		InstallNamespace:    installNamespace,
		IsManagementCluster: isManagementCluster,
	}

	if err := t.EnsureClusterSecret(ctx, &clusterRegisterConfig); err != nil {
		return microerror.Mask(err)
	}

	if err := t.ensureClusterRegistered(ctx, &clusterRegisterConfig); err != nil {
		return microerror.Mask(err)
	}

	if !controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
		patchHelper, err := patch.NewHelper(cluster, t.CtrlClient)
		if err != nil {
			return errors.WithStack(err)
		}
		controllerutil.AddFinalizer(cluster, key.TeleportOperatorFinalizer)
		if err := patchHelper.Patch(ctx, cluster); err != nil {
			t.Logger.Error(err, "failed to add finalizer.")
			return microerror.Mask(client.IgnoreNotFound(err))
		}
		t.Logger.Info("Successfully added finalizer.", "finalizer_name", key.TeleportOperatorFinalizer)
	}
	return nil
}

func (t *Teleport) EnsureClusterDeregistered(ctx context.Context, cluster *capi.Cluster) error {
	if controllerutil.ContainsFinalizer(cluster, key.TeleportOperatorFinalizer) {
		// Clean up secrets and configmaps for the cluster
		if err := t.DeleteClusterSecret(ctx, cluster); err != nil {
			return microerror.Mask(err)
		}
		if err := t.DeleteClusterConfigMap(ctx, cluster); err != nil {
			return microerror.Mask(err)
		}

		// De-register the cluster from teleport cluster
		registerName := key.GetRegisterName(t.Config.ManagementClusterName, cluster.Name)
		if cluster.Name == t.Config.ManagementClusterName {
			registerName = cluster.Name
		}
		if err := t.ensureClusterDeregistered(ctx, registerName); err != nil {
			return microerror.Mask(err)
		}

		// Remove the finalizer
		patchHelper, err := patch.NewHelper(cluster, t.CtrlClient)
		if err != nil {
			return errors.WithStack(err)
		}
		controllerutil.RemoveFinalizer(cluster, key.TeleportOperatorFinalizer)
		if err := patchHelper.Patch(ctx, cluster); err != nil {
			t.Logger.Error(err, "failed to remove finalizer.")
			return microerror.Mask(client.IgnoreNotFound(err))
		}
		t.Logger.Info("Successfully removed finalizer.", "finalizer_name", key.TeleportOperatorFinalizer)
	}
	return nil
}

func (t *Teleport) isClusterRegistered(ctx context.Context, registerName string) (bool, tt.KubeServer, error) {
	ks, err := t.Client.GetKubernetesServers(ctx)
	if err != nil {
		return false, nil, err
	}

	for _, k := range ks {
		if k.GetCluster().GetName() == registerName {
			return true, k, nil
		}
	}

	return false, nil, nil
}

func (t *Teleport) ensureClusterRegistered(ctx context.Context, config *ClusterRegisterConfig) error {
	isRegistered, _, err := t.isClusterRegistered(ctx, config.RegisterName)
	if err != nil {
		return microerror.Mask(err)
	}
	if isRegistered {
		t.Logger.Info("Cluster is registered in teleport.")
		return nil
	}
	t.Logger.Info("Registering cluster in teleport...")
	joinToken, err := t.GenerateJoinToken(ctx, config.RegisterName)
	if err != nil {
		return microerror.Mask(err)
	}

	err = t.InstallApp(ctx, &AppConfig{
		InstallNamespace:    config.InstallNamespace,
		RegisterName:        config.RegisterName,
		ClusterName:         config.ClusterName,
		JoinToken:           joinToken,
		IsManagementCluster: config.IsManagementCluster,
	})
	if err != nil {
		return microerror.Mask(err)
	}
	return nil
}

func (t *Teleport) ensureClusterDeregistered(ctx context.Context, registerName string) error {
	t.Logger.Info("Checking if cluster is registered in teleport...")
	exists, ks, err := t.isClusterRegistered(ctx, registerName)
	if err != nil {
		return microerror.Mask(err)
	}
	if !exists {
		t.Logger.Info("Cluster does not exist in teleport.")
		return nil
	}
	t.Logger.Info("De-registering cluster from teleport...")
	if err := t.deregisterCluster(ctx, ks); err != nil {
		return microerror.Mask(err)
	}
	t.Logger.Info("Cluster de-registered from teleport.")
	return nil
}

func (t *Teleport) deregisterCluster(ctx context.Context, ks tt.KubeServer) error {
	if err := t.Client.DeleteKubernetesServer(ctx, ks.GetHostID(), ks.GetCluster().GetName()); err != nil {
		return microerror.Mask(err)
	}
	return nil
}
