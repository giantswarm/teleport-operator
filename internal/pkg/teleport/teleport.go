package teleport

import (
	"context"

	"github.com/go-logr/logr"
	tc "github.com/gravitational/teleport/api/client"
	"sigs.k8s.io/controller-runtime/pkg/client"

	capi "sigs.k8s.io/cluster-api/api/v1beta1"
)

type Teleport struct {
	SecretConfig   *SecretConfig
	TeleportClient *tc.Client
	Namespace      string
}

type TeleportConfig struct {
	Log                 logr.Logger
	CtrlClient          client.Client
	Cluster             *capi.Cluster
	RegisterName        string
	InstallNamespace    string
	IsManagementCluster bool
}

func New(namespace string, secretConfig *SecretConfig) *Teleport {
	return &Teleport{
		SecretConfig: secretConfig,
		Namespace:    namespace,
	}
}

func (t *Teleport) IsClusterRegisteredInTeleport(ctx context.Context, config *TeleportConfig) (bool, error) {
	ks, err := t.TeleportClient.GetKubernetesServers(ctx)
	if err != nil {
		return false, err
	}
	for _, k := range ks {
		if k.GetCluster().GetName() == config.RegisterName {
			config.Log.Info("Cluster registered in teleport", "registerName", config.RegisterName)
			return true, nil
		}
	}

	config.Log.Info("Cluster not yet registered in teleport", "registerName", config.RegisterName)
	return false, nil
}

func (t *Teleport) DeleteClusterFromTeleport(ctx context.Context, config *TeleportConfig) error {
	ks, err := t.TeleportClient.GetKubernetesServers(ctx)
	if err != nil {
		return err
	}
	for _, k := range ks {
		if k.GetCluster().GetName() == config.RegisterName {
			if err := t.TeleportClient.DeleteKubernetesServer(ctx, k.GetHostID(), k.GetCluster().GetName()); err != nil {
				return err
			}
			config.Log.Info("Deleted cluster from teleport", "registerName", config.RegisterName)
		}
	}
	return nil
}
