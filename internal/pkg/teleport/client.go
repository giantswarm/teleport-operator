package teleport

import (
	"context"

	tc "github.com/gravitational/teleport/api/client"

	"github.com/giantswarm/microerror"
)

func (t *Teleport) GetTeleportClient() (*tc.Client, error) {
	ctx := context.TODO()

	// Create new teleport client
	teleportClient, err := tc.New(ctx, tc.Config{
		Addrs: []string{
			t.SecretConfig.ProxyAddr,
		},
		Credentials: []tc.Credentials{
			tc.LoadIdentityFileFromString(t.SecretConfig.IdentityFile),
		},
	})
	if err != nil {
		return nil, microerror.Mask(err)
	}

	_, err = teleportClient.Ping(ctx)
	if err != nil {
		return nil, microerror.Mask(err)
	}

	return teleportClient, nil
}
