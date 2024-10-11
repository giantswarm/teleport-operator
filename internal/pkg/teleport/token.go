package teleport

import (
	"context"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/gravitational/teleport/api/types"

	"github.com/giantswarm/microerror"

	"github.com/giantswarm/teleport-operator/internal/pkg/key"
)

func (t *Teleport) IsTokenValid(ctx context.Context, registerName string, token string, tokenType string) (bool, error) {
	tokens, err := t.TeleportClient.GetTokens(ctx)
	if err != nil {
		return false, microerror.Mask(err)
	}

	expectedRoles, err := key.ParseRoles(tokenType)
	if err != nil {
		return false, microerror.Mask(err)
	}

	for _, t := range tokens {
		if t.GetName() == token &&
			t.GetMetadata().Labels["cluster"] == registerName {
			// Check if the token has expired
			if !t.Expiry().IsZero() && t.Expiry().After(time.Now()) {
				// Check if the token has all the expected roles
				tokenRoles := t.GetRoles()
				if len(tokenRoles) != len(expectedRoles) {
					return false, nil
				}
				for _, role := range expectedRoles {
					if !containsRole(tokenRoles, role) {
						return false, nil
					}
				}
				return true, nil
			}
			return false, nil
		}
	}

	// If we didn't find a matching token, it's not valid
	return false, nil
}

func containsRole(roles []types.SystemRole, role string) bool {
	for _, r := range roles {
		if strings.ToLower(r.String()) == role {
			return true
		}
	}
	return false
}

func (t *Teleport) GenerateToken(ctx context.Context, registerName string, roles []string) (string, error) {
	tokenValidity := time.Now().Add(key.TeleportKubeTokenValidity)
	tokenRoles := key.RolesToSystemRoles(roles)

	token, err := types.NewProvisionToken(t.TokenGenerator.Generate(), tokenRoles, tokenValidity)
	if err != nil {
		return "", microerror.Mask(err)
	}
	// Set cluster label to token
	{
		m := token.GetMetadata()
		m.Labels = map[string]string{
			"cluster": registerName,
			"roles":   key.RolesToString(roles),
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
