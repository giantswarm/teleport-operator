package teleport

import (
	"context"

	tc "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"

	"github.com/giantswarm/microerror"
)

type Client interface {
	Ping(ctx context.Context) (proto.PingResponse, error)
	GetToken(ctx context.Context, name string) (types.ProvisionToken, error)
	GetTokens(ctx context.Context) ([]types.ProvisionToken, error)
	CreateToken(ctx context.Context, token types.ProvisionToken) error
	UpsertToken(ctx context.Context, token types.ProvisionToken) error
	DeleteToken(ctx context.Context, name string) error
}

func NewClient(ctx context.Context, proxyAddr, identityFile string) (Client, error) {
	teleportClient, err := tc.New(ctx, tc.Config{
		Addrs: []string{
			proxyAddr,
		},
		Credentials: []tc.Credentials{
			tc.LoadIdentityFileFromString(identityFile),
		},
		InsecureAddressDiscovery: true,
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
