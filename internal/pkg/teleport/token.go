package teleport

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	tt "github.com/gravitational/teleport/api/types"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsTokenValid(ctx context.Context, config *TeleportConfig, oldToken string) (bool, error) {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return false, err
	}
	for _, token := range tokens {
		if token.GetMetadata().Labels["cluster"] == config.RegisterName {
			if token.GetName() == oldToken {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
}

func (t *Teleport) GenerateToken(ctx context.Context, config *TeleportConfig) (string, error) {
	var (
		tokenValidity = time.Now().Add(key.TeleportTokenValidity)
		uniqueToken   = strings.Replace(uuid.NewString(), "-", "", -1)
	)

	token, err := tt.NewProvisionToken(uniqueToken, []tt.SystemRole{tt.RoleKube, tt.RoleNode}, tokenValidity)
	if err != nil {
		return "", err
	}
	// Set cluster label to token
	{
		m := token.GetMetadata()
		m.Labels = map[string]string{
			"cluster": config.RegisterName,
		}
		token.SetMetadata(m)
		if err := t.TeleportClient.UpsertToken(ctx, token); err != nil {
			return "", err
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
