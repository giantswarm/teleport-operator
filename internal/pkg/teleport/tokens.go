package teleport

import (
	"context"
	"fmt"
	"time"

	"github.com/giantswarm/microerror"
	tt "github.com/gravitational/teleport/api/types"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsTokenValid(ctx context.Context, oldToken string, registerName string) (bool, error) {
	{
		tokens, err := t.Client.GetTokens(ctx)
		if err != nil {
			return false, err
		}

		for _, t := range tokens {
			if t.GetMetadata().Labels["cluster"] == registerName {
				if t.GetName() == oldToken {
					return true, nil
				}
				return false, nil
			}
		}
		return false, nil
	}
}

func (t *Teleport) GenerateJoinToken(ctx context.Context, registerName string) (string, error) {
	joinToken, err := t.GetToken(ctx, registerName)
	if err != nil {
		return "", microerror.Mask(fmt.Errorf("failed to generate token: %w", err))
	}
	return joinToken, nil
}

func (t *Teleport) GetToken(ctx context.Context, registerName string) (string, error) {
	// Look for an existing token or generate one if it's expired
	tokens, err := t.Client.GetTokens(ctx)
	if err != nil {
		return "", err
	}

	for _, t := range tokens {
		if t.GetMetadata().Labels["cluster"] == registerName {
			return t.GetName(), nil
		}
	}

	// Generate a token
	expiration := time.Now().Add(key.TeleportJoinTokenValidity)
	token := randSeq(32)
	newToken, err := tt.NewProvisionToken(token, []tt.SystemRole{tt.RoleKube, tt.RoleNode}, expiration)
	if err != nil {
		return "", err
	}
	metadata := newToken.GetMetadata()
	metadata.Labels = map[string]string{
		"cluster": registerName,
	}
	newToken.SetMetadata(metadata)
	err = t.Client.UpsertToken(ctx, newToken)
	if err != nil {
		return "", err
	}

	return token, nil
}
