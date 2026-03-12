package teleport

import (
	"context"

	tc "github.com/gravitational/teleport/api/client"
	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"google.golang.org/grpc"

	"github.com/giantswarm/microerror"
)

type Client interface {
	Ping(ctx context.Context) (proto.PingResponse, error)
	// GetToken fetches a single provision token by name. This is the only token
	// read operation the operator uses; the Teleport role for this operator does
	// NOT need the `list` verb on the token resource.
	GetToken(ctx context.Context, name string) (types.ProvisionToken, error)
	// GetTokens is retained in the interface to satisfy the upstream Teleport
	// client type but is not called by the operator. Do not add calls to this
	// method — doing so would reintroduce the `list` requirement and re-enable
	// cross-tenant token enumeration.
	GetTokens(ctx context.Context) ([]types.ProvisionToken, error)
	CreateToken(ctx context.Context, token types.ProvisionToken) error
	UpsertToken(ctx context.Context, token types.ProvisionToken) error
	DeleteToken(ctx context.Context, name string) error
}

var NewClient = func(ctx context.Context, proxyAddr, identityFile string) (Client, error) {
	teleportClient, err := tc.New(ctx, tc.Config{
		Addrs: []string{
			proxyAddr,
		},
		Credentials: []tc.Credentials{
			tc.LoadIdentityFileFromString(identityFile),
		},
		DialOpts: []grpc.DialOption{
			grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(10 * 1024 * 1024)),
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
