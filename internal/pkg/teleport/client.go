package teleport

import (
	"context"
	"fmt"

	tc "github.com/gravitational/teleport/api/client"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

	"github.com/giantswarm/microerror"
)

func (t *Teleport) GetClient() (*tc.Client, error) {
	ctx := context.TODO()

	// Get a config to talk to the apiserver
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, microerror.Mask(fmt.Errorf("unable to get config to talk to the apiserver: %s", err))
	}

	// Create a new client
	if t.CtrlClient, err = client.New(cfg, client.Options{}); err != nil {
		return nil, microerror.Mask(fmt.Errorf("unable to create a new kubernetes client: %s", err))
	}

	if t.Config, err = t.GetConfigFromSecret(ctx); err != nil {
		return nil, microerror.Mask(fmt.Errorf("unable to create get config from secret: %s", err))
	}

	teleportClient, err := tc.New(ctx, tc.Config{
		Addrs: []string{
			t.Config.ProxyAddr,
		},
		Credentials: []tc.Credentials{
			tc.LoadIdentityFileFromString(t.Config.IdentityFile),
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
