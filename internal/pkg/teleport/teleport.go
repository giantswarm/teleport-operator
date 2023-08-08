package teleport

import (
	"context"

	"github.com/giantswarm/microerror"
	"github.com/go-logr/logr"
	tc "github.com/gravitational/teleport/api/client"
)

type Teleport struct {
	SecretConfig   *SecretConfig
	TeleportClient *tc.Client
	Namespace      string
}

func New(namespace string, secretConfig *SecretConfig) *Teleport {
	return &Teleport{
		SecretConfig: secretConfig,
		Namespace:    namespace,
	}
}

func (t *Teleport) IsClusterRegisteredInTeleport(ctx context.Context, log logr.Logger, registerName string) (bool, error) {
	ks, err := t.TeleportClient.GetKubernetesServers(ctx)
	if err != nil {
		return false, microerror.Mask(err)
	}
	for _, k := range ks {
		if k.GetCluster().GetName() == registerName {
			log.Info("Cluster registered in teleport", "registerName", registerName)
			return true, nil
		}
	}

	log.Info("Cluster not yet registered in teleport", "registerName", registerName)
	return false, nil
}

func (t *Teleport) DeleteClusterFromTeleport(ctx context.Context, log logr.Logger, registerName string) error {
	ks, err := t.TeleportClient.GetKubernetesServers(ctx)
	if err != nil {
		return microerror.Mask(err)
	}
	for _, k := range ks {
		if k.GetCluster().GetName() == registerName {
			if err := t.TeleportClient.DeleteKubernetesServer(ctx, k.GetHostID(), k.GetCluster().GetName()); err != nil {
				return microerror.Mask(err)
			}
			log.Info("Deleted cluster from teleport", "registerName", registerName)
		}
	}
	return nil
}
