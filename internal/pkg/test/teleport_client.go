package test

import (
	"context"
	"fmt"

	"github.com/gravitational/teleport/api/client/proto"
	"github.com/gravitational/teleport/api/types"
	"github.com/pkg/errors"
)

type FakeTeleportClientConfig struct {
	FailsPing   bool
	FailsGet    bool
	FailsList   bool
	FailsCreate bool
	FailsUpsert bool
	FailsDelete bool
	Tokens      []types.ProvisionToken
}

type FakeTeleportClient struct {
	failsPing   bool
	failsGet    bool
	failsList   bool
	failsCreate bool
	failsUpsert bool
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
		failsList:   config.FailsList,
		failsCreate: config.FailsCreate,
		failsUpsert: config.FailsDelete,
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
	token, ok := c.tokens[name]
	if ok {
		return token, nil
	}
	return nil, fmt.Errorf("mock teleport client: token with name %s does not exist", name)
}

func (c *FakeTeleportClient) GetTokens(ctx context.Context) ([]types.ProvisionToken, error) {
	if c.failsList {
		return nil, errors.New("mock teleport client failed to get tokens")
	}
	var tokens []types.ProvisionToken
	for _, token := range c.tokens {
		tokens = append(tokens, token)
	}
	return tokens, nil
}

func (c *FakeTeleportClient) CreateToken(ctx context.Context, token types.ProvisionToken) error {
	if c.failsCreate {
		return errors.New("mock teleport client failed to create token")
	}
	c.tokens[token.GetName()] = token
	return nil
}

func (c *FakeTeleportClient) UpsertToken(ctx context.Context, token types.ProvisionToken) error {
	if c.failsUpsert {
		return errors.New("mock teleport client failed to upsert token")
	}
	c.tokens[token.GetName()] = token
	return nil
}

func (c *FakeTeleportClient) DeleteToken(ctx context.Context, name string) error {
	if c.failsDelete {
		return errors.New("mock teleport client failed to delete token")
	}
	delete(c.tokens, name)
	return nil
}
