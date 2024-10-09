package teleport

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/gravitational/teleport/api/types"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsTokenValid(ctx context.Context, registerName string, oldToken string, tokenType string) (bool, error) {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return false, microerror.Mask(err)
	}
	for _, token := range tokens {
		if token.GetMetadata().Labels["cluster"] == registerName && token.GetMetadata().Labels["type"] == tokenType {
			if token.GetName() == oldToken {
				return true, nil
			}
			return false, nil
		}
	}
	return false, nil
}

func (t *Teleport) GenerateToken(ctx context.Context, registerName string, tokenType string) (string, error) {
	var (
		tokenValidity time.Time
		tokenRoles    []types.SystemRole
	)

	switch tokenType {
	case "kube":
		tokenValidity = time.Now().Add(key.TeleportKubeTokenValidity)
		tokenRoles = []types.SystemRole{types.RoleKube}
	case "kubeapp":
		tokenValidity = time.Now().Add(key.TeleportKubeTokenValidity)
		tokenRoles = []types.SystemRole{types.RoleKube, types.RoleApp}
	case "node":
		tokenValidity = time.Now().Add(key.TeleportNodeTokenValidity)
		tokenRoles = []types.SystemRole{types.RoleNode}
	default:
		return "", microerror.Mask(fmt.Errorf("token type %s is not supported", tokenType))
	}

	token, err := types.NewProvisionToken(t.TokenGenerator.Generate(), tokenRoles, tokenValidity)
	if err != nil {
		return "", microerror.Mask(err)
	}
	// Set cluster label to token
	{
		m := token.GetMetadata()
		m.Labels = map[string]string{
			"cluster": registerName,
			"type":    tokenType,
		}
		token.SetMetadata(m)
		if err := t.TeleportClient.UpsertToken(ctx, token); err != nil {
			return "", microerror.Mask(err)
		}
	}
	return token.GetName(), nil
}

func (t *Teleport) DeleteToken(ctx context.Context, log logr.Logger, registerName string) error {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return err
	}
	for _, token := range tokens {
		if token.GetMetadata().Labels["cluster"] == registerName {
			if err := t.TeleportClient.DeleteToken(ctx, token.GetName()); err != nil {
				return microerror.Mask(err)
			}
			log.Info("Deleted teleport node/kube join token for the cluster", "registerName", registerName)
			return nil
		}
	}
	return nil
}
