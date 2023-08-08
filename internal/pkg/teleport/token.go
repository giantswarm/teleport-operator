package teleport

import (
	"context"
	"time"

	"github.com/google/uuid"
	tt "github.com/gravitational/teleport/api/types"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsTokenValid(ctx context.Context, config *TeleportConfig, oldToken string, tokenType string) (bool, error) {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return false, microerror.Mask(err)
	}
	for _, token := range tokens {
		if token.GetMetadata().Labels["cluster"] == config.RegisterName && token.GetMetadata().Labels["type"] == tokenType {
			if token.GetName() == oldToken {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
}

func (t *Teleport) GenerateToken(ctx context.Context, config *TeleportConfig, tokenType string) (string, error) {
	var (
		tokenValidity time.Time
		tokenRole     tt.SystemRole
	)

	switch tokenType {
	case "kube":
		tokenValidity = time.Now().Add(key.TeleportKubeTokenValidity)
		tokenRole = tt.RoleKube
	case "node":
		tokenValidity = time.Now().Add(key.TeleportNodeTokenValidity)
		tokenRole = tt.RoleNode
	}

	token, err := tt.NewProvisionToken(uuid.NewString(), []tt.SystemRole{tokenRole}, tokenValidity)
	if err != nil {
		return "", microerror.Mask(err)
	}
	// Set cluster label to token
	{
		m := token.GetMetadata()
		m.Labels = map[string]string{
			"cluster": config.RegisterName,
			"type":    tokenType,
		}
		token.SetMetadata(m)
		if err := t.TeleportClient.UpsertToken(ctx, token); err != nil {
			return "", microerror.Mask(err)
		}
	}
	return token.GetName(), nil
}

func (t *Teleport) DeleteToken(ctx context.Context, config *TeleportConfig) error {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if token.GetMetadata().Labels["cluster"] == config.RegisterName {
			if err := t.TeleportClient.DeleteToken(ctx, token.GetName()); err != nil {
				return microerror.Mask(err)
			}
			config.Log.Info("Deleted join token from teleport", "registerName", config.RegisterName)
			return nil
		}
	}
	return nil
}
