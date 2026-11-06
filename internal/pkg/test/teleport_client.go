package test

import (
	"context"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/gravitational/trace"
	"github.com/pkg/errors"
)

type FakeTeleportClientConfig struct {
	FailsPing   bool
	FailsGet    bool
	FailsCreate bool
	FailsDelete bool
	Tokens      []types.ProvisionToken
}

type FakeTeleportClient struct {
	failsPing   bool
	failsGet    bool
	failsCreate bool
	failsDelete bool
	tokens      map[string]types.ProvisionToken
}

func NewTeleportClient(config FakeTeleportClientConfig) *FakeTeleportClient {
	tokens := make(map[string]types.ProvisionToken)
	if config.Tokens != nil {
		for _, token := range config.Tokens {
			tokens[token.GetName()] = token
		}
	}

	return &FakeTeleportClient{
		failsPing:   config.FailsPing,
		failsGet:    config.FailsGet,
		failsCreate: config.FailsCreate,
		failsDelete: config.FailsDelete,
		tokens:      tokens,
	}
}

func (c *FakeTeleportClient) Ping(ctx context.Context) (proto.PingResponse, error) {
	var err error
	if c.failsPing {
		err = errors.New("mock teleport client failed ping")
	}
	return proto.PingResponse{}, err
}

func (c *FakeTeleportClient) GetToken(ctx context.Context, name string) (types.ProvisionToken, error) {
	if c.failsGet {
		return nil, errors.New("mock teleport client failed to get token")
	}
	if name == "" {
		return nil, trace.BadParameter("missing token name")
	}
	token, ok := c.tokens[name]
	if ok {
		return token, nil
	}
	return nil, trace.NotFound("token %s not found", name)
}

func (c *FakeTeleportClient) CreateToken(ctx context.Context, token types.ProvisionToken) error {
	if c.failsCreate {
		return errors.New("mock teleport client failed to create token")
	}
	c.tokens[token.GetName()] = token
	return nil
}

func (c *FakeTeleportClient) DeleteToken(ctx context.Context, name string) error {
	if c.failsDelete {
		return errors.New("mock teleport client failed to delete token")
	}
	if name == "" {
		return trace.BadParameter("missing token name")
	}
	if _, ok := c.tokens[name]; !ok {
		return trace.NotFound("token %s not found", name)
	}
	delete(c.tokens, name)
	return nil
}
